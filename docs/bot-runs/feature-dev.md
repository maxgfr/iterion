# Featurly — `feature-dev` run bilans

Autonomous end-to-end feature development: plan → act → `/simplify` →
prepare_commit → alternating Claude/GPT review-fix loop → commit, in an isolated
`worktree: auto`. See [bots/feature-dev/](../../bots/feature-dev/).

## 2026-06-13 — sandbox-doctor static-binary check (run 019ec149)

- Status: **failed to converge — implementation correct, review loop is broken for
  new-file features.**
- Versions: bot feature-dev 0.1.0 · iterion f247f360
- Method: `POST /api/runs`, `feature_prompt` = add a static-binary check to
  `iterion sandbox doctor` (warn when the host iterion is dynamically linked — the
  exact trap that broke Seki). `--merge-into none`, default `workspace_dir`
  (worktree-isolated ✅, `.iterion/worktrees/019ec149...`, safe under watchexec).
  Backends: claude_code opus (plan/act/simplify/fix_claude/prepare_commit) + claw
  gpt-5.5 (reviewer_gpt/fix_gpt). **101.7k tokens, $4.95, 507 steps, review_loop
  10/15 — cancelled (non-convergent).**

### Value (the implementation is genuinely good)
- `act` produced a **correct, well-documented** feature: `pkg/cli/sandbox_binary.go`
  with `iterionBinaryIsStatic(path)` detecting static-vs-dynamic via the ELF
  `PT_INTERP` program header, a focused `_test.go`, and the `sandbox doctor`
  integration in `sandbox.go`. The doc comment even cites `addClawBinaryMount` and
  the precise `exec: … no such file or directory` failure mode. Salvageable from the
  preserved (cancelled-run) worktree.

### Findings / misses
1. **SEVERE — feature-dev cannot converge on a feature that ADDS files.** The
   reviewer anchor protocol correctly says "diff `git diff HEAD`, NOT `HEAD^…HEAD`"
   (so a reviewer doesn't conclude "feature not implemented" off the base commit).
   But **`git diff HEAD` omits untracked files** — and `act` creates new files
   without `git add`ing them. So the helper + test (`pkg/cli/sandbox_binary.go`,
   `…_test.go`) were `??` untracked, invisible to the reviewers' `git diff HEAD
   --name-only`. The GPT reviewer **correctly** rejected every pass:
   *"the helper and focused unit test are still untracked … the committable tracked
   diff references `iterionBinaryIsStatic` without including its implementation or
   the required test."* The `fix_*` agents can't resolve it (the files already
   exist; the real gap is staging), so it loops to `review_loop(15)` and dies. This
   almost certainly hits **any** review loop that anchors on `git diff HEAD` for a
   change that adds files (feature-dev, possibly Billy/branch-improve-loop and Doki).
2. **Cost of non-convergence:** $4.95 / 101k tokens / 507 steps burned on 10 passes
   that could never pass — the loop has no "is this failing for a structural reason
   I can't fix?" escape, it just re-runs the fixer against an unfixable blocker.

### Engine hardening / fix (recommended — needs a validated re-run)
- Make untracked new files visible to the review diff. Cleanest: a deterministic
  `git -C <wt> add -N .` (intent-to-add) **before** the anchor diff, so `git diff
  HEAD` shows new files as additions (full content) without fully staging them; the
  existing `prepare_commit`'s `git add -- <files>` still does the real staging at
  commit. Equivalent belt-and-suspenders: have `act`/`fix_*` `git add` new files
  when they create them. Apply the same to every loop bot that diffs `git diff HEAD`.
- The canonical asymptote guidance in
  [docs/workflow_authoring_pitfalls.md] / CLAUDE.md ("reviewers MUST diff `git diff
  HEAD`, not `HEAD^…HEAD`") is now **extended** with the untracked-files caveat
  (CLAUDE.md, asymptote section).
- **Not patched in this pass** (a careful multi-spot reviewer-prompt change that
  needs its own validated Featurly run); tracked here as the next feature-dev fix.

### Lessons for next run
- Before trusting a feature-dev run, check the worktree's `git status`: if `act`
  left `??` untracked files, the review loop will never converge until they're
  staged — that's the bug above, not a bad implementation.
- The implemented feature here (sandbox-doctor static-binary warning) is worth
  salvaging by hand from the worktree — it directly prevents the dynamic-binary
  trap that cost this campaign two Seki failures.
