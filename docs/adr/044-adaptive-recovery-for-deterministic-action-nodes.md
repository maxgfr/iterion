# ADR-044 — Adaptive recovery for deterministic ACTION nodes (gates stay deterministic)

Status: **proposed / exploratory** (2026-06-23) — reflection, no implementation yet.

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
