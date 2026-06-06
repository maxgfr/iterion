# ADR-011: whole_improve_loop fragments the workspace by package into per-pass token-budgeted chunks

- **Status**: Accepted
- **Date**: 2026-06-02
- **Authors**: devthejo
- **Code context**: [`bots/whole-improve-loop/main.bot`](../../bots/whole-improve-loop/main.bot)
  (`snapshot_chunk` tool, `streak_check` compute, `max_review_chunk_tokens` var),
  [`bots/whole-improve-loop/README.md`](../../bots/whole-improve-loop/README.md)

## Context

`whole_improve_loop` is the whole-repository alternating Claude/GPT
review→fix loop. Its reviewer nodes were pointed at the entire workspace
(`Review the code in {{vars.workspace_dir}}`) under a system prompt that
ordered them to "review ALL the code in the workspace," armed with
filesystem tools. On a workspace the size of iterion itself
(~195k LoC of Go alone; the chunker measures **~2.2M estimated tokens of
source across ~1,200 files in ~100 package directories**), a reviewer
that faithfully obeys that instruction reads files until it blows the
model's context window and the run dies with `context_length_exceeded`
— on the **first** iteration. The bot could not be dogfooded on iterion,
which blocked the "iterion-on-iterion" loop (issue #12).

The fix had to (a) bound each review so context can never be exhausted,
(b) still cover the whole workspace, (c) keep the existing convergence
contract (cross-family double-approval terminates the loop), and (d) make
`max_review_chunk_tokens` (default 30000) the knob — while respecting two
hard properties of the iterion DSL/runtime discovered during design:

1. **There is no dynamic fan-out.** `router fan_out_all` dispatches to a
   *statically declared* set of edges; you cannot map one reviewer node
   over an N-element runtime list where N depends on repo size
   ([docs/routers.md](../routers.md)).
2. **The per-run cost/time budget is real.** The workflow caps itself at
   `max_cost_usd: 60` / `max_duration: 2h`. Reviewing ~2.2M tokens of
   source needs ~100 premium-model reviews *per full sweep*.

## Decision

Insert a deterministic `snapshot_chunk` **tool** node (Python, no LLM) as
the workflow entry and the loop-return target. Each pass it:

- enumerates source files (excluding deps/build/hidden dirs and
  non-source extensions), groups them by **directory = package boundary
  first**, then bin-packs whole packages into chunks of at most
  `max_review_chunk_tokens` estimated tokens (~4 bytes/token); a single
  package larger than the budget is split by **file-size budget** (the
  "package boundary first, then file size" heuristic);
- selects **exactly one** chunk and emits its full source inline
  (`chunk_content`, bounded) plus the file list and
  `chunk_index`/`num_chunks` metadata;
- rotates the selected chunk via a **persisted monotonic cursor**
  (`.whole_improve_loop.cursor` at the workspace root).

The reviewer reviews **one chunk per pass** from the inline content
(tools allowed only for narrow cross-references, never bulk reads). The
`streak_check` compute node was made **coverage-aware**: it counts
*consecutive fully-clean passes* (`clean_streak`) and only fires `stop`
when that streak reaches `num_chunks + 1` (a full sweep of distinct
chunks, +1, with the family alternating throughout) — collapsing to the
original `2` when `num_chunks <= 1`. This is the chunked analogue of "two
consecutive cross-family approvals," and it makes a single clean chunk
**structurally unable** to terminate the loop. The fixer is unchanged in
structure: it still inherits the same-family reviewer's session (which
already holds that pass's chunk) and fixes that chunk's blockers.

The merge/consolidation the acceptance criteria call for is realised as
this **accumulation across passes**: `cumulative_scanned_areas` unions
the audited surfaces and the coverage-gated streak is the single
consolidated signal the convergence test reads — rather than a within-one-pass
fan-out-and-merge of all chunks.

## Trade-offs

| Dimension | Per-pass single chunk + coverage-gated streak (chosen) | Per-pass fan-out of ALL chunks + merge (rejected) |
|---|---|---|
| Context safety | Inline `chunk_content` ≤ `max_review_chunk_tokens` (hard char cap); the reviewers' own read tools stay open and are steered off bulk-reads by prompt, not hard-capped | Same per chunk |
| Fits the DSL | Yes — sequential loop, no dynamic fan-out needed | **No** — needs dynamic fan-out over a runtime-sized chunk list, which iterion lacks |
| Cost per *full sweep* | ~`num_chunks` reviews, **spread across passes/runs** under the $60/2h cap | ~`num_chunks` reviews **every pass** → ~100 premium calls/pass on iterion → blows $60/2h in one pass |
| Convergence signal | `clean_streak >= num_chunks+1` (full alternating sweep; both families participate, per-chunk dual-family accrues across sweeps — see Corrections) | `approved = AND(all chunks)` per pass |
| Cross-run progress | Persisted cursor advances coverage across re-dispatches | N/A (can't get through one pass) |
| Reuses repo precedent | docs-refresh's deterministic-prepass + chunk-aware streak | none |

The decisive factor is that the literal "review per-chunk then merge into
one consolidated review **per pass**" interpretation is **cost-infeasible
on the very workspace the acceptance criteria name as the target**: at
~100 chunks and premium models it exceeds the workflow's own
`max_cost_usd`/`max_duration` budget before a single pass finishes. The
chosen design spends the same total review effort but amortises it across
passes (and, for very large repos, across re-dispatched runs via the
persisted cursor), which is exactly how docs-refresh (issue #2) already
makes its parallel chunking story tractable.

## Alternatives considered

### 1. Per-pass fan-out over all chunks, then a merge node (the literal AC)
Rejected: needs dynamic fan-out the runtime doesn't provide, and even
simulated as a sequential inner loop it performs ~`num_chunks` premium
reviews *per outer pass*, blowing the declared cost/time budget on
iterion-scale repos. Recorded as the rejected alternative because the AC
text leans this way; the budget constraint (itself part of the spec)
overrides it.

### 2. Reuse `__scan-shards` child runs (one `iterion run` per chunk)
The existing `cmd/iterion/scan_shards.go` fan-out spawns child runs and
aggregates. Rejected as the primary mechanism: it shards by **file
count**, not token budget, and is not package-aware; each child boots its
own sandbox; and threading the alternating cross-family streak contract
through N child runs per pass is far more machinery (and far less
testable) than a deterministic in-process chunker. The token-aware
package planner here is the piece that would have to be added to
`__scan-shards` anyway; co-locating it in the bot keeps the change to one
file.

### 3. Loop-counter chunk rotation instead of a persisted cursor
Use `loop.review_loop.iteration % num_chunks` to pick the chunk.
Rejected: the loop counter **resets every run**, so a re-dispatched run
would re-review chunks `0..review_loop` forever and never reach a large
repo's tail. The persisted cursor (a neutral, gitignore-able dotfile,
same convention as docs-refresh's cache) makes coverage advance across runs
and removes any dependency on loop-counter-vs-snapshot timing.

## Consequences

- **No more `context_length_exceeded` on the first iteration.** Each
  review sees one bounded chunk; the acceptance command
  (`iterion run bots/whole-improve-loop/main.bot --var workspace_dir=.`)
  starts reviewing chunk 0 (~0.5–28k tokens) instead of inhaling the repo.
- **Convergence is coverage-gated.** `stop` needs a full clean sweep of
  every chunk (`clean_streak >= num_chunks + 1` consecutive blocker-free
  passes) with the reviewing family alternating across chunks, so **both
  families participate in the terminating sweep**; a single clean chunk
  can no longer end the loop. Per-chunk *dual*-family coverage accrues
  across successive sweeps, not within one — see the 2026-06-02 entry
  under Corrections for why the gate is deliberately not strengthened to
  require it. Verified against the real expression evaluator (nil-safety
  on the first pass, threshold scaling, blocker reset).
- **Very large repos converge across passes/runs, not in one pass.** When
  `num_chunks` exceeds the `review_loop` bound, a single run makes
  bounded, context-safe progress and the persisted cursor lets the next
  dispatch continue; full single-run convergence needs a higher
  `max_review_chunk_tokens` (fewer chunks) and/or budget. This is
  documented in the README "Large workspaces" note and is the honest
  ceiling — no chunking topology reviews ~2.2M tokens with premium models
  inside $60.
- **A new state file** `.whole_improve_loop.cursor` is written at the
  workspace root; operators should gitignore it (added to iterion's
  `.gitignore`). Deleting it restarts coverage from chunk 0.
- **Cross-package issues can span chunks.** A bug whose two halves live in
  different packages may not be visible within a single chunk; reviewers
  may follow a specific cross-reference with read tools, but the design
  trades some whole-program visibility for tractability — the same
  trade-off docs-refresh accepts.
- **`max_review_chunk_tokens` is the dial.** Raise it to review more per
  pass (fewer chunks, faster convergence, larger prompt); lower it for a
  smaller-context model.

## Corrections

### 2026-06-02 — cross-family coverage wording (and the rejected gate)

The original Consequences bullet and the README claimed `stop` needs a
clean sweep of every chunk **"by both families."** That overstated the
guarantee and contradicted this ADR's own Decision section (which
correctly says the family *alternates* throughout). The wording is now
corrected in both places. The mechanics: the rotation cursor advances
exactly one chunk per pass and the `round_robin` router advances exactly
one family per pass, so a terminating window of `num_chunks + 1`
consecutive clean passes reviews each chunk by exactly **one** family —
only the single wrap chunk is revisited (by the *same* family when
`num_chunks` is even, by the *other* when odd). The real, delivered
guarantee is a *full alternating clean sweep* in which **both families
participate**; dual-family coverage of any individual chunk accrues
across *successive* sweeps.

**Decision on the fix (Option A over Option B).** Two fixes were on the
table: (A) correct the docs to state the true guarantee, or (B)
strengthen the gate to require every chunk be reviewed clean by **both**
families before `stop`. We chose **A** and deliberately did **not** adopt
B, for the same budget reason this ADR already turned on:

- B requires the clean streak to span both parities for every chunk —
  roughly `2 * num_chunks` consecutive blocker-free passes (more, once
  the even-`num_chunks` parity-lock on the wrap chunk is accounted for).
- On the named target (iterion, `num_chunks ≈ 101`) that is ~202
  consecutive clean passes, vastly beyond the `review_loop(15)` bound and
  the `max_cost_usd: 60` / `max_duration: 2h` budget. `clean_streak` is
  run-local (only the rotation *cursor* persists, not the streak), so
  re-dispatch cannot accumulate it across runs either. B would therefore
  make the flagship target **strictly less** convergent — the opposite of
  issue #12's goal.
- Aggregate cross-family-per-sweep (both families review, alternating) is
  the budget-feasible analogue of the pre-chunking "two consecutive
  cross-family approvals," and it is what the code already implements.

The rejected alternative (a per-chunk dual-family gate) is recorded here
so the trade-off is explicit: we accept weaker per-chunk cross-family
coverage **within a single sweep** in exchange for a convergence target
that is actually reachable under the declared cost/time budget. Teams
that need strict per-chunk dual-family review on a *small* repo
(`2 * num_chunks <= review_loop` bound) can raise
`max_review_chunk_tokens` until the repo fits in one or two chunks, which
restores the original semantics exactly.

### 2026-06-02b — the streak is now PERSISTED across runs (stop was unreachable on iterion)

**The bug.** The 2026-06-02 entry above already conceded, in passing,
that `clean_streak` is run-local and "re-dispatch cannot accumulate it
across runs either." That concession was fatal and under-weighted: it
made the headline goal of this ADR — *converge on iterion* —
**unreachable**, not merely slow. Concretely, `clean_streak` lived only
in `streak_check`'s run-local output (a self-reference,
`outputs.streak_check.clean_streak`), so:

- a single run does at most `review_loop` passes (hardcoded `15`), so
  `clean_streak` could never exceed 15 in one run; and
- the streak reset to 0 on every (re-)dispatch (only the rotation
  *cursor* persisted).

The stop threshold on iterion is `num_chunks + 1 ≈ 102`. Since
`15 < 102` and the streak never carried across runs, **`stop` could
never fire on iterion under any number of re-dispatches** — every run
exhausted `review_loop` and routed to `fail`. The "converge across
passes/runs" claim in Consequences and the README "Large workspaces"
note were therefore **false**: the cursor advanced *coverage* but
nothing advanced the *streak*. The only configurations that made `stop`
reachable (`num_chunks ≤ 14`, i.e. `max_review_chunk_tokens ≥ ~160k`)
re-introduced the exact `context_length_exceeded` this ADR exists to
remove. The two acceptance criteria were mutually unsatisfiable on the
named target.

**The fix (chosen: persist the streak; supersedes the "converge across
runs" prose above).** The single JSON state file at the workspace root —
formerly `.whole_improve_loop.cursor`, now `.whole_improve_loop.state` —
carries BOTH the rotation cursor AND the `clean_streak`. `snapshot_chunk`
seeds the streak from disk at the start of a (re-)dispatched run and
re-persists it every pass (the post-review value is handed back on the
loop-return edge). `streak_check` now reads its base from
`input.persisted_clean_streak` (the snapshot's value) instead of the
run-local self-reference. So the `num_chunks + 1` clean-sweep threshold
now ACCUMULATES across re-dispatches, which is what makes cross-family
approval able to terminate the loop on a repo whose `num_chunks` exceeds
one run's pass budget. (Reading the base from the snapshot also removes
the `if(prev, prev+1, 1)` nil-vs-0 ambiguity that conflated a first pass
with a mid-run reset.)

**Alternatives rejected.**

1. *Raise `review_loop` so one run converges.* The runtime supports a
   templated bound; we DID expose it as `max_review_passes` (default 15)
   for small/mid repos and bigger budgets. But on iterion (~101 chunks)
   a single converging run needs ~102 premium reviews — far past
   `max_cost_usd: 60` / `max_duration: 2h`. So a tunable bound *helps*
   but cannot be the whole fix; cross-run persistence is what stays
   inside budget. (This is the "and/or budget" path the 2026-06-02
   Consequences gestured at — now real, but as a *multi-run* mechanism,
   not a single-run one.)
2. *Persist a per-chunk clean SET keyed by content hash, stop when the
   set covers every chunk (the reviewer's "more robust variant").* This
   converges faster — a late blocker invalidates only its own chunk, not
   the whole consecutive streak — and mirrors docs-refresh's anchor cache
   more closely. Deferred, not adopted now: it is a larger change to the
   `snapshot_chunk` planner (per-file SHAs threaded as coverage) carrying
   more bug surface for a fix that needed to land minimally and be
   verifiable without a live premium run. The consecutive-streak model
   is faithful to the pre-chunking "two consecutive cross-family
   approvals" and is now *correct* (just not optimally fast). Recorded as
   the leading future optimization.

**Consequences delta.**

- `stop` is reachable on iterion: a sequence of re-dispatches (or manual
  re-runs of the acceptance command) accumulates the clean sweep and
  exits via `streak_check -> done`. The per-run `-> fail` on
  `review_loop` exhaustion is retained as the "not converged yet" signal
  (changing it risks the no-silent-success guarantee and is orthogonal);
  the operator/dispatcher re-runs until a run exits via `stop`.
- The state file is now `.whole_improve_loop.state` (JSON), not
  `.whole_improve_loop.cursor`. `.gitignore` updated. Deleting it
  restarts BOTH coverage and the streak from 0.
- New var `max_review_passes` (default 15) wires the `review_loop` cap
  via the runtime's templated-loop-bound support; an int var default
  always coerces, so the bound never degrades to 0.
- Verified against the real expression evaluator (clean-streak build,
  blocker reset, threshold fire at `num_chunks+1`, `num_chunks<=1`
  collapse to 2) and the chunker's state read/write (cross-run resume,
  float/empty/literal `STATE_IN` handling, dotfile not counted as
  source). The earlier "converge across passes/runs" claim is now
  accurate; the README "Large workspaces" note is corrected to match.

### 2026-06-02c — the persisted streak was not crash-safe (false convergence)

**The bug.** The 2026-06-02b persistence fix made `stop` *reachable* on
iterion, but it left a **silent false-convergence** hole: the loop could
exit via `streak_check -> done` (workspace declared clean) while a chunk
still held an un-reviewed/un-fixed blocker. Mechanism:

- `snapshot_chunk` persisted `{cursor: cursor + 1, clean_streak: <base>}`
  at the **start** of each pass — i.e. it advanced the rotation cursor
  *eagerly*, before the reviewer ran, while the `clean_streak` it wrote
  was the pre-verdict base.
- On a non-clean pass, `streak_check` reset `clean_streak` to 0, but that
  reset only reached disk on the **next** snapshot (carried on the
  loop-return edge). If the run ended in between — overwhelmingly the
  common case, because the `fix_*` LLM node failing on
  budget/rate-limit/cancel is exactly what the design routes to `fail` +
  re-dispatch — disk was left with the cursor advanced **past** the
  blocker chunk but `clean_streak` at the stale pre-blocker value (≥2).
- On re-dispatch the entry snapshot seeded the streak from that stale-high
  disk value and resumed at the chunk *after* the blocker. The remaining
  window (`threshold − stale_streak`, which is `< num_chunks` whenever
  `stale_streak ≥ 2`) never wrapped back to the blocker chunk, so `stop`
  fired without it ever being re-reviewed. Reproduced on a 3-chunk repo:
  clean, clean, blocker-then-crash → disk `{cursor:3, clean_streak:2}` →
  re-dispatch converged after reviewing only the *other two* chunks. This
  re-introduced the very "silent empty success" the `-> fail` routing
  exists to prevent, through the persisted state.

Root cause: the **cursor advance** (must happen every pass, verdict-
independent) and the **streak value** (must reflect the verdict) were
persisted *together, pre-verdict*, by the only filesystem-writing node
(`snapshot_chunk`). A non-clean verdict's reset was therefore not durable
before a run-ending failure, leaving an internally inconsistent
`{cursor, clean_streak}` pair on disk.

**The fix (chosen: couple the cursor advance to the verdict).**
`snapshot_chunk` now persists the `{cursor, clean_streak}` pair it is
**using** this pass — no eager `cursor + 1`. The advance rides
`streak_check`, which emits `next_cursor = input.cursor + 1`; **every**
loop-return edge hands BOTH `incoming_clean_streak` AND `incoming_cursor`
back to the snapshot. The two state fields now move in one atomic write,
so the on-disk pair is always consistent: `clean_streak` counts the clean
passes for chunks strictly *before* `cursor`, and `cursor` names the chunk
about to be (re-)reviewed. A crash on any pass leaves `cursor` pointing
**at** the un-verdicted chunk, so the re-dispatch re-reviews it instead of
skipping it. Verified by replaying the chunker + `streak_check` logic: the
crash scenario above no longer converges while the blocker persists (the
blocker chunk is re-reviewed every rotation and resets the streak), and a
fully-clean repo still converges at exactly `num_chunks + 1`.

**Alternatives rejected.**

1. *Synchronous "mark dirty" write node on the `streak_check -> fix_*`
   edges* (write `clean_streak: 0` the instant a blocker is found, before
   the fixer runs). This closes the **common** vector (a graceful `fix_*`
   failure) but only *shrinks* — does not eliminate — the window: a hard
   kill in the sub-second gap between `streak_check` and the mark-dirty
   write still strands a stale streak. It also inserts a node into the
   load-bearing `streak_check -> fix_*` **session-inheritance** path
   (`_session_id` is threaded there so the fixer reuses the reviewer's
   session and prompt cache), risking that cost optimisation. The chosen
   fix is airtight *and* leaves the session edges untouched.
2. *Per-chunk content-hash clean SET* (the 2026-06-02b "more robust
   variant", deferred). Still the leading future direction — its failure
   mode is uniformly safe *omission* and it removes the cursor/streak
   coupling entirely — but it is a larger rewrite of the `snapshot_chunk`
   planner (per-chunk SHAs threaded as coverage, set-coverage stop test)
   with more bug surface than this minimal, simulation-verifiable change.
   The consecutive-streak model, once the cursor advance is verdict-
   coupled, is *correct*; the set model would make it *faster to
   converge*. Recorded again as the next optimisation, not a correctness
   prerequisite.

**Consequences delta.**

- No silent false convergence: a crash/`fail` on a non-clean pass leaves
  the cursor on the un-verdicted chunk, so re-dispatch re-reviews it; the
  consecutive-streak guarantee now holds across the crash boundary.
- `snapshot_output` gains a `cursor` field and `streak_state` gains
  `next_cursor`; the reviewer→`streak_check` edges carry `cursor` and all
  four `-> snapshot_chunk` loop-return edges carry `incoming_cursor`. No
  new nodes; the state-file format is unchanged (`{cursor, clean_streak,
  num_chunks}`), so existing `.whole_improve_loop.state` files remain
  compatible (a stale `cursor+1` from a pre-fix run costs at most one
  extra re-review).
- The false-safety comment in `snapshot_chunk` ("never a false
  convergence …") is removed and replaced with the accurate crash-safety
  rationale; the README "Large workspaces" note is updated to match.
