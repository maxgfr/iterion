# Billy — branch-improvement validation

## 2026-06-14 — re-validation on a clean clone + good dead-code judgment (run 019ec5bc)

- Status: **validated.** Re-ran in the C082 worktree studio (non-watchexec) on a
  clean iterion clone with a synthetic `billy-test` branch: one added file
  `pkg/log/billy_demo.go` exporting `WriteMarker`, which had no godoc AND
  swallowed the `os.Create` error. `base_ref=main`, `merge_into=none`.
- Result: cross-family review loop **converged** (review → fix → re-review →
  streak), `Run finished`, committed to storage branch
  `iterion/run/feral-crash-duskvane-127c` (`final_commit 9c5a5891`), not merged
  (merge_into=none respected).
- Value — **correct judgment, not a cosmetic fix.** Rather than just adding godoc
  + handling the error, Billy recognized `WriteMarker` as *unreferenced dead code
  that swallows an error* and **removed it** (`refactor(log): remove unused
  WriteMarker demo helper`, "No code in the tree calls it, so remove the dead
  code."). That's the right call — exactly what a demanding reviewer should do.
- Finding (minor, non-fatal): `fix_claude` (claude_code) emitted one
  `Tool error: StructuredOutput — No such tool available: StructuredOutput`
  before recovering and producing its output normally. The agent appears to try a
  `StructuredOutput` tool that isn't registered in the claude_code delegate — a
  wasted step, same broad family as the Devy claude_code-structured-output gap.
  Worth wiring/​silencing, but it did NOT block convergence.
- Convergence machinery (shared with Willy) is reference-correct; this run
  re-confirms it + the asymptote (no oscillation) on a fresh target.

---

**Status:** validated end-to-end (2026-06). **Scope of this report:** the
capability and the engineering hardening it drove. The target here is
iterion's own repository (the `feat/cloud-control-plane` epic), so target
details are included.

## Summary

Billy (the `branch-improve-loop` bot) was exercised against a real, large
branch — a ~7000-line / 42-chunk epic — and demonstrably:

1. **converges** a cross-family review/fix loop on a big diff (monotonic
   decrease to zero blockers + a two-family approval streak), rather than
   oscillating or stalling;
2. **finds real, high-value issues** the human author missed, and
   **authors complete fixes** for them — including an ADR for a design-level
   change;
3. **drives the full pipeline** plan_chunks → alternating cross-family review
   (Claude ↔ GPT) → same-family fix → `streak_check` → `prepare_commit` →
   semantic `commit_changes`, and stops at the asymptote.

The exercise also hardened the engine: **one significant runtime bug was
root-caused and fixed** while driving real runs — without it the GPT family
could not review a diff this size at all.

## What was validated

- The loop: `plan_chunks (deterministic diff measure + chunking) →
  round_robin reviewer (claude-opus-4-8 ↔ openai/gpt-5.5) → family-matched
  fixer → streak_check (2 consecutive opposite-family approvals) →
  prepare_commit → commit_changes`.
- **Chunked review at scale:** a 42-chunk diff, each reviewer reading chunks
  one at a time then merging into one whole-diff verdict (cross-family
  approval is on the whole diff, never chunk-by-chunk).
- **Convergence to an asymptote**, not oscillation — the bot settled into a
  stable approved state and committed.
- **Both backends working through the whole run**, including the GPT family on
  the local ChatGPT-forfait path.

## Method

A single end-to-end run (`019eb168`) on `feat/cloud-control-plane`, ~7000
lines of diff. Reviewers/fixers: `claude-opus-4-8` (Claude Code) and
`openai/gpt-5.5` (claw, ChatGPT-forfait). Sandboxed (per-run container),
`--merge-into none` so the result lands on a storage branch for review.
Budget raised to 8h/250$ for this run (the 2h/60$ default is too small for a
diff this size); convergence took ~2h of effective run time.

## Result

Converged: status `finished`, commit `1ffc4bc` —
`fix(secrets,memory): enforce bot-secret binding egress and isolate bot
memory by id`, 745 insertions / 84 deletions across 30 files, build + tests
green. Fast-forward-merged into the epic, then merged to `main`.

Convergence trajectory (blockers per verdict):

```
claude 2 → gpt 2 → claude 1 → gpt 1 → claude 1 → gpt 1
→ claude 0 (approved) → cross-family streak → commit
```

A slight oscillation near the end (GPT re-raised one blocker before settling)
is within the accepted asymptote behaviour; the `prior_pushback` /
`previous_scanned_areas` feedback kept verdicts from re-litigating resolved
items in a loop.

## What Billy found and fixed

Genuine issues in the reviewed epic, fixed with tests:

- `cloudpublisher` did not persist `RepoURL`/`RepoSHA`/`BotID`, breaking
  cloud/webhook **resume** and bot-bound secret resolution. Fixed across the
  publisher, `queue/types`, and the run store.
- `secretguard` did not intersect a bot-secret binding's egress hosts. Fixed,
  with ADR 018.
- Bot memory was not isolated by bot id. Fixed (`fsstore`, `scope`).
- Binding-route validation tightened; new tests added.

## Engineering hardening (the enabler)

Before the fix, `gpt-5.5` on the ChatGPT forfait died on the 42-chunk review
with `context_length_exceeded` — not a fundamental limit but a bug: the
forfait's effective context window is smaller than the model's advertised
1.05M, so claw's preemptive compaction (sized to the advertised window) never
triggered in time, and nothing reacted to the backend's rejection.

Fix: **reactive force-compaction** — on a context-window rejection the tool
loop force-compacts the running history to a shrinking target
(256k→128k→64k→32k, independent of the advertised window) and retries.
Surfaced as an `llm_retry`, reusing claw's existing pure compactor. With it,
Billy ran for hours with both GPT nodes (reviewer **and** fixer) and zero
context-overflow deaths.

## Operational resilience

The run absorbed several transient infrastructure interruptions — network
drop, ChatGPT-forfait cap, and an intermittent sandbox bootstrap flake — via
delegate-level network retries and an auto-resume loop that relaunches from
the checkpoint (no progress lost) until convergence. None were the
context-overflow bug; all were absorbed without operator intervention.
