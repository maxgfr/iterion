# ADR-044 — Adaptive recovery for deterministic ACTION nodes (gates stay deterministic)

Status: **accepted (partial)** (2026-06-23) — the Verified Action quad is
implemented on `tool` nodes: layers **A** (postcondition), **E** (policy
spine), and **C** (self-repair) ship enabled; **B** (agent recovery) is
wired but opt-in and OFF by default (`recovery.max_agent_attempts: 0`).
Layer **D** (robust primitive library, e.g. `iterion __commit`) and
extending the quad to `compute` nodes are deferred follow-ups. Originally
proposed / exploratory the same day (kept below).

## Implementation (2026-06-23)

The quad is `goal` + `command`/`script` (recipe) + `postcondition` +
`policy` on a `tool` node — all optional and fully backward-compatible (a
node with no `postcondition` behaves exactly as before: recipe only, exit
code = success).

- **DSL/IR**: new optional `tool` properties `goal:`, `postcondition:`,
  `policy:` (`required` | `recover` | `best_effort`), and a `recovery:`
  block (`max_repair_attempts`, `max_agent_attempts`, `model`,
  `agent_tools`). Parser + AST + IR + unparse round-trip them.
- **Runtime ladder** lives in
  [pkg/backend/model/executor_verified_action.go](../../pkg/backend/model/executor_verified_action.go):
  idempotent-skip → recipe → self-repair → agent-recovery → policy, with the
  postcondition re-checked at every rung as the single source of truth
  (success is keyed on it, never the recipe exit code). Self-repair's
  corrected command and the postcondition check are emitted as `tool_called`
  events; the per-node outcome is a `node_verified_action` event.
- **Anti-Goodhart firewall** is enforced at compile time, not left to
  convention: **C103** (invalid policy), **C104** (recovery without a
  postcondition — no truth oracle), **C105** (recovery attached to a *gate*
  where `recipe == postcondition`), **C106** (recovery bounds under a
  non-`recover` policy — dead config). A gate stays the degenerate quad
  (`recipe == postcondition`, no rungs 3–4, `policy: required`).
- **Demonstration**: `bots/secured-renovacy`'s `commit_changes` node now
  carries a postcondition ("working tree has no relevant uncommitted changes
  left, caches excluded") + `policy: recover`, so it self-heals past the two
  brittle failures cited below instead of hard-blocking.

---

Status (original): **proposed / exploratory** (2026-06-23) — reflection, no implementation yet.

## Context

Deterministic `tool`/`compute` nodes are iterion's anti-Goodhart / anti-façade
backbone: `scan_health`, coverage gates, `streak_check`, `cap_findings` (see
[docs/workflow_authoring_pitfalls.md](../workflow_authoring_pitfalls.md)). They are
fast, cheap, auditable, and never drift — that is *why* review loops are
trustworthy.

But on the **action** side they are brittle, and brittleness hard-*blocks* a run.
Evidence from the 2026-06-23 re-test, on a single node, twice in a row:

- `secured-renovacy` commit step failed on a stray Go module cache overflowing
  Node's 1 MB `execSync` maxBuffer (`…1050864 more characters`, exit 1);
- after that was patched, it failed again on `:!__pycache__` — git short-form
  pathspec magic mis-parsing a `_`-leading pattern (`Unimplemented pathspec magic
  '_'`, exit 128).

Each needed a code patch. An LLM asked to "commit the relevant changes, excluding
caches" would have adapted past both without a human in the loop.
`sec-audit-source`'s voters similarly hard-failed on one missing structured-output
field. The deterministic path's strength (no drift) is its weakness (no
adaptation → hard block on the unanticipated).

## The crux: not all non-LLM nodes are equal

- **GATE / verification nodes** (judge coverage, enforce the approval streak, check
  scan health, cap findings) — MUST stay deterministic. Their whole purpose is to
  be **un-gameable by the LLM** (the Goodhart firewall). An LLM fallback here
  *reintroduces* the façade risk the gate exists to prevent. → never LLM-fallback a
  gate.
- **ACTION / side-effect nodes** (commit, git ops, file writes, invoking a scanner,
  mechanical transforms) — have **no gaming surface**; their only failure mode is
  brittleness. → this is precisely where adaptivity / fallback helps.

So the answer to "should non-LLM nodes have an LLM fallback?" is: **yes for action
nodes, no for gates.** The reflex to "just let a model handle it" is right for the
commit and wrong for the coverage gate.

## Approaches (for ACTION nodes), most-promising first

A. **Adaptive action + deterministic postcondition** (recommended where a clean
   postcondition exists). An `agent` achieves the goal (commit), then a *cheap*
   deterministic node verifies the postcondition (e.g. "exactly one commit on
   `iterion/run/*` whose tree excludes `go/`, `target/`, `__pycache__`"). You get
   adaptability AND safety — and the verify is mechanical (a property check, not a
   quality judgment), so it stays Goodhart-safe. This is the cleanest split:
   adaptive *doing*, deterministic *checking*.

B. **Deterministic-first, LLM-recovery-on-failure** — a DSL affordance like
   `on_error: agent("commit the changed manifests/lockfiles/vendor, exclude caches")`.
   The fast deterministic command stays the default; only on a hard exit does the
   runtime hand the agent `{command, stderr, goal}` + real tools to finish, then
   continue. 95% cheap/predictable path, 5% adaptive recovery instead of a block.
   Minimal, general, opt-in. (Generalizes the engine's existing resumable-failure
   idea into an in-run recovery step.)

C. **LLM-repairs-command, determinism-executes** (self-healing, auditable middle
   ground). On failure an LLM proposes a *corrected command* (it sees the error);
   the runtime re-runs it deterministically, bounded attempts. Execution stays
   deterministic + visible in events (you see the fixed command); the model only
   fixes the recipe, never does the side effect blind. Good for "the command was
   almost right" cases (both Renovacy failures were exactly this).

D. **Robust-primitive library** (complements A–C). Most brittleness is *ad-hoc
   shell*: git pathspecs, JSON shape handling, buffer limits, glob portability
   (cf. the dash/bash pitfalls doc). Ship tested iterion primitives — e.g. an
   `iterion __commit --exclude-caches` subcommand, a json-shape normalizer,
   a `__git-status-clean` — that nodes call instead of fragile inline shell.
   Robust by construction; shrinks the surface A–C have to cover.

E. **Per-node failure semantics** — a node declares `required | best_effort |
   recover`. Today most hard-fail; the commit should `recover` (B/C), an optional
   enrichment should `best_effort`, a security gate stays `required`. Also
   subsumes the campaign's "inconsistent rate-limit handling" finding (some nodes
   graceful-pause, others hard-fail — make it a declared policy).

## Recommendation

1. Keep gates deterministic (the Goodhart firewall) — do **not** LLM-ify them.
2. For action nodes: prefer (A) where a postcondition is expressible (commit,
   file-producing steps); add (B)/(C) as the general opt-in recovery for the rest.
3. Build (D) primitives for the recurring brittle operations — *commit with
   cache-exclusion* is the obvious first one (it failed twice this session).
4. Prototype on the commit node (the proven pain point), measure, then generalize.

## Consequences

Determinism is kept exactly where it guarantees trust (gates) and softened exactly
where it only adds brittleness (actions). Bots get materially more reliable —
fewer hard blocks on unanticipated inputs — *without* weakening the anti-façade
guarantees. Cost: a recovery agent invocation on the rare failure path, and a new
DSL surface (`on_error:` / failure-semantics) to design carefully so it can't be
abused to paper over a real gate failure (recovery is for actions, never gates).

## Synthesis — the "Verified Action" pattern (mix, don't choose)

The five approaches are **not alternatives; they are layers that compose**, and
they sit at different levels:

- **D (robust primitives)** = the foundation (fewer edge cases become failures).
- **A (deterministic postcondition)** = the load-bearing spine (the truth oracle).
- **C (self-repair) then B (agent recovery)** = graduated recovery rungs.
- **E (failure semantics)** = the policy that bounds it.

The load-bearing insight: **A is what makes B and C safe.** On their own, an LLM
recovery / self-repair reintroduces the façade risk (the model can *claim* it
committed). With a cheap deterministic postcondition checked after *every* rung,
adaptive recovery becomes trustworthy — the agent cannot fake success past a
property check. So A is not "one option among five"; it is the enabler that lets
you mix in the adaptive ones without losing the anti-Goodhart guarantee.

### The unifying model

Every node becomes a quad: **goal + recipe + postcondition + policy.**
- `goal` — the outcome, in words (fuel for the agent rungs).
- `recipe` — the deterministic command (the fast path; what self-repair fixes).
- `postcondition` — a cheap deterministic property check (the single source of truth).
- `policy` — `required | recover | best_effort`.

Runtime escalation (cheapest → most adaptive, postcondition is truth at each rung):
1. **Idempotent skip** — postcondition already met? do nothing (resume-safe).
2. **Recipe** (robust primitive). Postcondition met → done. *(the ~95% path)*
3. **Self-repair (C)** — LLM proposes a *corrected recipe* from the error; re-run
   deterministically, bounded. Postcondition met → done. *(auditable: the fixed
   command is in the events; no blind side effect)*
4. **Agent recovery (B)** — agent achieves the `goal` with real tools, bounded.
   Postcondition met → done.
5. **Policy (E)** — still unmet: `required`→fail, `best_effort`→warn+continue.

### Why this is the most robust + flexible + clean + universal

- **Universal** — the quad subsumes *every* node type as a degenerate case:
  - a **gate** = `recipe == postcondition`, no rungs 3–4, `policy: required`
    (pure determinism — the Goodhart firewall, unchanged);
  - a **transform/compute** = recipe + schema-as-postcondition + self-repair;
  - a **side-effecting action** (commit, file write, scanner) = full escalation;
  - even an **LLM node** = the agent *is* the recipe, schema *is* the
    postcondition — which is exactly what the review judges and sec-audit's
    missing-field retry already do. One model for the whole graph.
- **Robust** — *postcondition-as-truth, not exit-code-as-truth.* Exit codes lie
  (`nothing to commit` exits 1 though the goal may be met; a command can exit 0
  yet not achieve the outcome). Keying success on the postcondition + the
  idempotent skip makes retries/resumes naturally correct. Both Renovacy failures
  this session were "recipe almost right" → caught at rung 3 with zero human/code.
- **Flexible** — authors opt into exactly the rungs they want; a bot can ship
  recipe-only (today's behaviour) and add a postcondition later, then recovery.
- **Clean** — one mental model (generate → verify) for the entire engine.

### The deepest framing

iterion is *already* "adaptive generate + deterministic verify" — that is what a
review loop is (agent generates, judges + gates verify). Action nodes are the
**lone exception**: deterministic generate, *no* verify → brittle and unguarded.
This pattern simply brings action nodes onto the same spine the rest of the engine
already runs on. The synergy isn't five features bolted together; it's making one
existing principle universal.

Recommended first cut: implement A+E as the spine (postcondition + policy on action
nodes), add C (self-repair) as the cheap first recovery rung, B (agent) as the
opt-in second rung, and grow D (primitives) for the operations that keep breaking
(commit-with-cache-exclusion first). Prototype on the commit node.
