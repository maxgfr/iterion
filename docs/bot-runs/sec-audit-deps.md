# Depsy — `sec-audit-deps` run bilans

Universal supply-chain / SCA auditor: enumerates installed deps per ecosystem,
runs heuristics + CVE baseline, LLM-reviews, emits findings to the board.
Read-only (no code edits). See [bots/sec-audit-deps/](../../bots/sec-audit-deps/).

> **Known status (CLAUDE.md):** the heuristic scanner layer is still a scaffold
> (runs the real CVE scanners but discards their output, tracked native:3a81df64);
> a run self-labels with a "⚠ Coverage" banner. It is enumerate + LLM-review only
> until that lands.

## 2026-06-13 — iterion self-audit dogfood (run 019ec17e)

- Status: **inconclusive — hung at `enumerate_deps`, cancelled.**
- Versions: bot sec-audit-deps 0.1.0 · iterion (post-fixes, static binary installed)
- Method: `POST /api/runs`, `severity_threshold=high`, sandboxed
  (`iterion-sandbox-sec`). Launched in **parallel** with a Featurly re-run.
  Read-only (no `remediate`/`patch_author`/`worktree:` — confirmed safe: it can't
  self-kill or pollute the tree the way Seki's remediation did).
- Result: the static-binary + `backendIsClaw` fixes meant the sandboxed claw
  runner started (no "iterion not found" — note Depsy uses literal `backend:
  "claw"`, so it was only ever blocked by the static-binary issue, not the
  env-template one). But `enumerate_deps` (the first node, claw/gpt-5.5)
  **hung**: ~28 min with a single in-flight LLM call, `last_seq` stuck at 6, no
  tool calls, no retries logged. Cancelled.

### Findings / misses
1. **`enumerate_deps` stalls (medium — reliability, needs a clean repro).** One
   gpt-5.5 enumeration call hung with no progress/retry. Plausible contributors:
   (a) resource contention — two sandbox-sec-class containers + multiple LLM
   calls were running in parallel (Seki had just finished, Featurly2 was live);
   (b) the enumeration prompt ingesting large manifests (the bot's own skill warns
   against reading lockfiles — worth checking it doesn't); (c) a transient claw /
   ChatGPT-forfait stall. Re-run **alone** (no parallel sandbox bots) to isolate.
2. Did not reach `llm_review` or board emit, so the SCA path + the known scaffold
   gap (#native:3a81df64) remain **unvalidated** on iterion this pass.

### Lessons for next run
- Run Depsy **alone**, not alongside another sandbox-sec bot — the parallel load
  is the most likely cause of the `enumerate_deps` stall.
- It is read-only and worktree-free → safe under `task studio:dev` (unlike Seki's
  remediation), so no special isolation needed; just give it dedicated resources.
- Still gated behind the documented scaffold caveat: treat output as
  enumerate + LLM-review, not a complete dependency audit, until native:3a81df64.
