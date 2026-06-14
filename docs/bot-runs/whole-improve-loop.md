# Willy — `whole-improve-loop` run bilans

Whole-repository alternating Claude/GPT review-fix loop. Reviews the workspace in
per-package chunks, fixes blockers in place, converges on two consecutive
cross-family approvals. See [bots/whole-improve-loop/](../../bots/whole-improve-loop/).

> Convergence machinery (`alt` round-robin → `reviewer_*` → `streak_check` →
> `fix_*`) is **shared with Billy** (`branch-improve-loop`), whose full
> cross-family convergence to an asymptote is validated in
> [branch-improve-loop.md](branch-improve-loop.md). This page covers Willy's
> whole-repo specifics.

## 2026-06-14 — scope_globs shipped + pkg/store hardening + fixer-placeholder finding (run 019ec7ed, cancelled-for-value)

- Status: **partial by design** — cancelled mid-sweep once it had produced its
  value (real fixes + clear findings) rather than run to convergence on a tree
  the fixer had silently broken. Converted directly into repatriated commits.
- Versions: bot whole-improve-loop 0.3.0 **+ new `scope_globs` var** · iterion
  base `2707ea2f`, fixes `ec2752ca..3be59a70` (worktree `worktree-willy-improve`).
- Method: CLI `iterion run` (a separate process — dodges the `task studio:dev`
  watchexec self-kill), `--var scope_globs=pkg/store` (the new var), into the
  operator's visible store (`/.iterion`, studio :4891). Backends: reviewer/fix
  `claude_code` opus-4-8 (max) + `claw` openai/gpt-5.5 (high) via ChatGPT
  forfait. `sandbox-full:edge`. Workspace = the worktree (Willy has no
  `worktree: auto`, so it edits its `workspace_dir` directly).
- Result: NOT run to convergence (cancelled after 3 chunks, ~$3.79, ~30 min).
  `scope_globs` pruned pkg/store to **4 chunks / 51 files / loop_max 19**
  (whole-repo iterion is **29 chunks / loop_max 44** — the cost the var fixes).
  chunk 0 clean (claude); chunk 1 → **4 real blockers** (gpt) → fix_gpt; chunk 2
  clean (claude). Repatriated by hand to the worktree branch.

### Value
- **`scope_globs` works end-to-end** — fixes the focused-run cost finding from the
  019ec598 bilan below. A focused pkg/store pass cost **~$3.79** instead of paying
  the whole-repo sweep (~$20-60 to crawl to the chunks you care about). This is the
  way to dogfood Willy affordably: scope to a package, get a converging run for a
  few dollars.
- **4 genuine pkg/store production-readiness blockers**, all verified real against
  the code, repatriated as `a79ffa76` (+ `hardening_test.go` regression tests the
  fixer's own plan called for but never wrote):
  - **B1** run dir/run.log created `0755`/`0644` (logs hold prompts/outputs/secrets →
    world-readable) + path built before run-ID validation → private `dirPerm`/`filePerm`
    + validate-first; `TeeRunLog(storeRoot, runID)`.
  - **B2** `AppendEvent`/`scanMaxSeqLocked` skipped `SanitizePathComponent`
    (traversal-defense asymmetry vs Load*/Artifact/Interaction).
  - **B3** `CreateRun` clobbered an existing `run.json` (run-ID reuse/race reset
    status/checkpoint) → exclusive create (`WriteFileAtomicNew` hard-link, `fs.ErrExist`).
  - **B4** a torn final JSONL line after a crash lost the first post-resume event →
    separate-with-newline repair.

### Findings / misses (bot + engine)
1. **fix_gpt returns a "work in progress" placeholder and ships unverified edits.**
   The fixer edited 5 files (9.3 min, 29k tokens) but its final turn produced empty
   output (`raw_output_len: 0`, `formatting_pass_used: true`), so iterion's
   formatting pass synthesized `applied=false` + "validating and fixing". Two
   regressions rode through unverified: **B4** `ReadAt` on a write-only fd (EBADF →
   broke *every* event append; 6 test failures) and **B1** a missed caller in
   `pkg/cli/resume.go` (build break). Root cause: the **review** prompt forbids
   provisional verdicts; the **fix** prompt did not. Fixed — FINALIZE guard in
   `fix_system` (`3be59a70`): no placeholder + self-verify with the project's own
   build/test + update ALL callers.
2. **The loop cannot catch a fixer that breaks the build (no deterministic post-fix
   gate).** Reviewers review their chunk's *source*; a cross-file build break
   (`resume.go`, outside the pkg/store scope) or a runtime-only bug (B4) is invisible,
   so the loop rebuilt a clean streak on a broken tree and would have
   **converged-on-broken**. The fix is the CLAUDE.md "deterministic gate" pattern:
   run the repo's OWN build/test as a `tool`/`compute` gate after fixes / before
   `commit_changes`, degrading the run if red. Universality-constrained (the
   build/test command is per-repo) → skill-guided detect + deterministic gate, like
   sec-audit-source's `scan_health`. **Recommended follow-up — not yet implemented.**
3. **scope_globs ↔ out-of-scope callers**: a scoped review can't see a signature
   change's callers outside the scope. The FINALIZE "update ALL callers" clause
   mitigates from the fixer side; the build gate (2) is the deterministic backstop.

### Engine / repo hardening produced
- `ec2752ca` + `deec5543` — `scope_globs` (feature + README); `f3df9cc7` — catalog regen.
- `a79ffa76` — pkg/store B1-B4 hardening + 4 regression tests.
- `3be59a70` — `fix_system` FINALIZE guard (no placeholder + self-verify).

### Lessons for next run
- Don't trust fix_gpt's `applied` flag or summary — **always run a build/test gate on
  the worktree before repatriating** (until the deterministic gate node lands).
- gpt-5.5 is a strong **reviewer** (4 real, well-analyzed blockers) but an unreliable
  **fixer** at the budget ceiling (placeholder + unverified edits). Until the build
  gate exists, prefer routing fixes to `claude_code` even for gpt-found blockers.
- For a clean convergence demo, scope tighter and raise `max_review_passes`; the
  persisted streak carries multi-run convergence regardless.

## 2026-06-14 — convergence machinery re-confirmed + path-scope finding (run 019ec598, cancelled)

- Status: **partial** (machinery confirmed; cancelled before the scoped edit —
  by design, see finding). Run on a clean iterion clone via the C082 worktree
  studio (non-watchexec, so no self-kill), `improvement_prompt` scoped to
  "pkg/log/ only", `merge_into=none`.
- Machinery: **confirmed healthy.** `alt` round-robin → `reviewer_claude`/
  `reviewer_gpt` → `streak_check` → `snapshot_chunk` turned correctly; reviewers
  emitted clean cross-family verdicts and `streak_check` accumulated approvals (4
  chunks swept, `review_loop=2`). No oscillation, no crash (Willy has a python
  state node but it does NOT parse json arrays from env, so it's immune to the
  Seki-class shape bug).
- **Finding — no path-scope glob → focused runs pay full-repo cost.** A
  `pkg/log/`-only `improvement_prompt` does NOT prune the chunk set: Willy still
  chunks the WHOLE repo and the reviewers no-op every non-pkg/log chunk
  (`"No action required... zero pkg/log/ source files"`) at ~$0.5/review/chunk.
  iterion has ~30+ packages, so a single-package focus would burn ~$30 of review
  to reach the one relevant chunk. `improvement_prompt`/`scope_notes` are prose
  (the WHAT), not a path filter (the WHERE). Recommended enhancement: add a
  `scope_globs` var (like sec-audit-source's `code_scope_globs`) that prunes the
  chunk plan, so focused improvements skip irrelevant packages. Cancelled here
  once the machinery + this finding were clear, to avoid the full-repo spend.
- Note: Willy's improvement *value* (catching/fixing a real dropped error) was
  already validated in the 2026-06-13 run below; this run targeted convergence +
  the scope behaviour, not re-proving value. Willy does not emit to the board.
- Lessons for next run: for a single-package improvement, either accept the
  full-repo sweep cost or use a different tool; pushing for a `scope_globs` var
  is the real fix. Whole-repo axes (e.g. "all error handling") are Willy's
  intended sweet spot, where the full sweep is correct.

## 2026-06-13 — bounded error-handling dogfood (run 019ec0c8)

- Status: **partial — core value validated, full convergence NOT reached** (the run
  killed itself, see finding #1).
- Versions: bot whole-improve-loop 0.3.0 · iterion 9197bcfd (v0.14.0)
- Method: launched via Studio `POST /api/runs`, scoped to a low-risk axis
  (`improvement_prompt` = surgical Go error-wrapping / nil-checks; `scope_notes`
  = minimal diffs; `max_review_passes=3`), `--merge-into none`, default
  `workspace_dir`. Backends: `claude_code` opus-4-8 (reviewer/fix), `claw` gpt-5.5
  (other family). `sandbox-full:edge`. ~3.7 min, ~$1.15, ~18k tokens counted before
  the run was cancelled.
- Result: `snapshot_chunk` (chunked iterion into **22 chunks / 1515 files / 5.8M
  est tokens**) → `alt` → `reviewer_claude` (found a real blocker) → `streak_check`
  → `fix_claude` (applied a correct fix) → **cancelled by a watchexec-triggered
  studio restart** (`error: "server drained: studio process shutting down"`,
  `failed_resumable` at `fix_claude`, review_loop=1).

### Value (the core loop works and finds real issues)
- **Reviewer found a genuine, on-axis bug**: `cmd/iterion/scan_shards.go`
  (`dispatchCloud`) had `req, _ := http.NewRequestWithContext(...)` — a silently
  dropped request-construction error. Precise `file (func)` localisation.
- **Fixer applied a correct, surgical fix**: `req, err := …; if err != nil {
  r.Error = …; return }`, matching the file's existing error-handling style.
  Compiles + `go test ./cmd/iterion` green. **Integrated to main as `4c525a6e`.**
- So Willy's value proposition (cross-family review finds real issues; fixer makes
  correct surgical edits) is demonstrated even though the run didn't finish.

### Findings / misses
1. **Willy self-kills under `task studio:dev` (CRITICAL — dogfood infra).** Willy
   edits the **live main working tree** (it has `sandbox:` but **no `worktree:
   auto`**, and no per-run worktree was created — confirmed `.iterion/worktrees/`
   empty for this run). `task studio:dev` runs the backend under
   `watchexec -r -e go -w cmd -w pkg -w vendor`. So the instant `fix_claude` wrote
   `cmd/iterion/scan_shards.go`, watchexec restarted the studio backend, which
   drained the in-flight run → `context canceled` → `failed_resumable`. **Any
   code-editing bot that touches `cmd/`/`pkg/` on the live tree will be cancelled by
   its own edits under the dev server.** Mitigations: run such bots against a
   non-watchexec studio (built `iterion studio`/`server`), or via a CLI
   `iterion run` in an independent process, or on an out-of-tree workspace copy.
2. **No worktree isolation (design tension — engine/bot).** Willy mutates the
   operator's actual checkout and (by design) leaves the edits uncommitted for
   review — but with no isolation it (a) pollutes the live tree, (b) self-destructs
   under file-watchers (#1), (c) risks losing edits on any restart. Billy
   (`branch-improve-loop`) and Featurly (`feature-dev`) use `worktree: auto` +
   a commit step. **Recommendation:** give Willy `worktree: auto` + a commit-on-
   convergence step (consistent with Billy), or at minimum document loudly that it
   edits the live tree. ADR-level decision, not a quick patch.
   **Evaluated 2026-06-13 — deferred.** The clean `worktree: auto` move has a real
   conflict: Willy's cross-run convergence relies on `.whole_improve_loop.state`
   (cursor + clean_streak) persisted at the **workspace root** to amortize the
   num_chunks-deep sweep across re-dispatches (issue-#12 / ADR-011). A `worktree: auto`
   worktree is created fresh from HEAD and **removed on finalize**, so that state would
   vanish each run → cross-run streak amortization breaks (every run re-sweeps from
   cursor 0). Doing it correctly means relocating the state off the ephemeral worktree
   (run-store or parent repo) **and** adding a commit step — a genuine ADR, not a patch.
   Since #1 is also solvable operationally (CLI launch / non-watchexec studio), the
   worktree change is deferred pending that ADR rather than rushed.
3. **Chunk grouping can exceed `max_review_chunk_tokens` ~7× (coverage).** Chunk 0
   was **218K est tokens / 149 files** against the 30K default budget; the renderer
   then hard-caps content at `budget*4+4096` (~124K chars), emitting
   `"... [chunk content truncated at the char cap] ..."` — so files grouped past the
   cap are **silently unreviewed** even though they count in `file_count`. The
   per-chunk grouping (by directory) doesn't split a group that overflows the
   budget. Worth bounding chunk size to the budget (or splitting oversize groups).

### Engine hardening
- `cmd/iterion/scan_shards.go` dropped-error fix — committed `4c525a6e`.
- Findings #1–#3 are recommendations (watchexec-incompat documented in CLAUDE.md;
  worktree-isolation + chunk-budget are deferred design/engine follow-ups).

### Lessons for next run
- **Do not dogfood Willy (or any live-tree code-editing bot) under
  `task studio:dev`** — its own edits trip watchexec and cancel the run. Use a
  non-watchexec studio or a CLI launch in a separate process.
- A whole-iterion convergence run is heavy (22 chunks / 5.8M tokens) and won't reach
  the cross-family asymptote under a small `max_review_passes`. For a convergence
  validation, point Willy at a **bounded** workspace (as Billy was pointed at a
  bounded branch diff), and raise the budget.
- Willy's reviewer/fixer quality is high; its weak points are *operational*
  (isolation + watcher interaction), not the LLM loop itself.
