[← Bot runs](README.md)

# adr-cartograph (Adry) — bilans

ADR cartographer + completeness auditor. Read-only on code, reasonably
idempotent: observes what the code implements and emits committable Nygard
ADRs in `docs/adr/`, plus a feature-completeness audit; files optional
handoff issues to `adr-rechallenge` (re-challenge) and `feature-gap-fill`
(gap completion). Template: docs-refresh (Doki).

## 2026-06-14 — finding #6 convergence tuning (runs 019ec2f3, 019ec4ec)

Two prompt-level fixes for #6, each validated by a live re-run on the
aligned tree (8 ADRs committed, 0 ADR-drift, coverage 100%):

- `7546d0d1` — `streak_check.stop` was stricter than its own `approved`
  passthrough (it omitted the low-confidence clause); aligned it so a
  low-confidence rejection is non-blocking (CLAUDE.md asymptote rule).
  **run4 (019ec2f3) still failed** — the blocker was `confidence: high`,
  not low, so the quick-win mis-aimed.
- `61e995bb` — the real root cause: the persistent high-confidence blocker
  was a **GAP** (the survey's file-diff-payload completeness finding), and
  both reviewers were treating the unfixable gap as a blocker ("file an
  issue for / address this gap"). Adry is read-only on code → it cannot
  close a gap → permanent oscillation. `review_system` rule 7: gaps are
  NEVER blockers (confirm-real → ride the deterministic handoff path to
  `feature-gap-fill`; only ADR drift / orphans / format defects block).

**Result: partial.** Rule 7 measurably helped — reviewers now sometimes vote
`approved=true, blocker_count=0` (they never did before) — but **run5
(019ec4ec) still exhausted `review_loop(10/10)`**: the LLM reviewers are
stochastic, and one still occasionally raises a gap as a blocker
(`bot-marketplace-shallow-clone`) despite rule 7, resetting the cross-family
streak. A prompt instruction alone is not a reliable gate.

**Structural completion (next step, NOT yet done):** stop passing `gaps`
into the reviewers' `review_input` entirely — let gaps flow from
`build_manifest` straight to `prepare_commit`'s handoff, never through the
review loop. With the reviewers blind to gaps (and rule 6 calibrating ADR
prose), an aligned tree has nothing to block on → it converges. This removes
the gap-blocker source *deterministically* instead of relying on the model
to obey rule 7. Needs a `review_input` schema/edge change + one validation
run. **Firm stop taken here** (5 Adry runs this campaign; survey ~$3.58 +
~10 min each is the floor since `survey_code` always runs before the
deterministic aligned-check).

## 2026-06-13 — first dogfood, scoped pkg/git (runs 019ec1f8 + 019ec25f)

- **Status: partial** — core machinery validated end-to-end and 2 real bugs
  fixed, but the bot does **not yet converge autonomously** (finding #6).
- **Versions:** bot 0.1.0 · iterion @ `f3289632` (worktree `worktree-adr-bot-suite`, unmerged)
- **Method:** `iterion run bots/adr-cartograph/main.bot --var code_scope_globs='pkg/git/**'`,
  forfait forced (`ITERION_OPENAI_USE_OAUTH=1`, API keys unset), claude_code
  opus-4-8 (scan/survey/reviewer_claude/fix_claude/prepare_commit) + claw
  openai/gpt-5.5 (reviewer_gpt/fix_gpt). No sandbox, no worktree:auto (the
  repo-root sha-cache must persist across runs). Scoped to one decision-rich
  package to bound cost.
- **Result:** **did not converge** — both the authoring run (019ec1f8) and a
  re-run on the now-aligned tree (019ec25f) exhausted `review_loop (10/10)`
  and hit the `fail` backstop. Survey ≈ $3.58 and ~10 min each (pkg/git, 14
  files, tool_max_steps 80). The review loop never reached the cross-family
  double-approval streak (see #6). **But** the run did the valuable work: it
  authored **8 genuinely good ADRs** (`docs/adr/020`–`027`, committed
  `b60106ba`) recording real pkg/git decisions — arg/path-injection
  hardening, IsAncestor-via-merge-base, empty-tree-SHA root baseline,
  C-locale stderr classification, NUL-framed parsing, working-tree (not
  index) panel semantics, main-repo-root gitdir resolution, git-CLI-shell-out
  vs go-git. Format matched the repo's Nygard convention exactly.
- **Value:** high — produced 8 committable ADRs from a cold read of pkg/git,
  with accurate Context/Decision/Alternatives/Consequences + a Re-challenge
  hook each. The survey's `is_mechanic` self-critique correctly flagged 0
  mechanical decisions (all 8 were real). scan_adrs correctly detected the
  historical `002-*` duplicate and computed `next_adr_number`.

### Findings
| # | Finding | Severity | State |
|---|---|---|---|
| 1 | `build_manifest` passed survey JSON via raw `{{!input.x}}` wrapped in literal quotes — LLM prose (apostrophes/parens) broke `bash -c` (`syntax error near ')'`) | real bug | **FIXED** `1bbca1c0` (drop `!` + quotes → `{{input.x}}` routes through shellEscapeValue). docs-refresh shares the latent pattern (only passes apostrophe-free tokens) → cross-bot pitfall added to [workflow_authoring_pitfalls.md](../workflow_authoring_pitfalls.md) |
| 5 | `build_manifest` computed coverage from the **frozen** `scan_adrs.adrs` (entry snapshot), so ADRs the fixers authored mid-loop were invisible → `coverage_pct` froze at 71% < 80% → the gate was unreachable → loop exhausted with 8 good ADRs on disk | real bug, convergence-breaking | **FIXED** `f3289632` (re-glob `docs/adr/*.md` each pass, parse `../../<code>` refs). Proven deterministically + LIVE on run3: coverage 71%→**100%**, drift 8→0 |
| 6 | Even with coverage=100% (aligned tree, 0 drift), the review loop **never reaches cross-family double-approval** — the two reviewers keep finding prose blockers on the 8 ADRs and oscillate to `review_loop(10/10)` → `fail` | real, **OPEN** | the bot cannot self-converge yet; see Lessons |
| 2 | `fix_claude` (claude_code, output-schema+tools → two-pass) hit `StructuredOutput — No such tool available` in Pass 1 (it's a Pass-2 mechanism), then recovered | benign engine quirk, pre-existing (all fix-nodes; `claude_code.go:341`) | note only |
| 3 | `enforce_fix_scope` `git checkout -- <p>` reverts ALL tracked changes outside `docs/adr/`, incl. pre-existing uncommitted WIP (it ate my own mid-run hot-fix of main.bot) | known loop-bot behavior (docs-refresh idem) | mitigation: commit fixes BEFORE resume (in HEAD = not "changed"); don't hot-edit tracked files during a run |
| 4 | `reviewer_gpt` → 401 OpenAI "token is expired" mid-run (forfait `~/.codex` 10 days stale) | infra | operator re-authed codex → resumed on forfait. See workspace memory `project_openai_oauth_token_invalidation` |

### Engine / bot hardening
- `1bbca1c0` — shell-safe data passing in build_manifest (finding #1).
- `f3289632` — build_manifest re-globs docs/adr so coverage tracks authored ADRs (finding #5).
- `b60106ba` — the 8 dogfood-authored pkg/git ADRs (kept for review).

### Lessons for next run (finding #6 is the gate to production)
The review loop must converge to an asymptote; right now it does not. The
coverage gate (#5) is satisfied but the **cross-family double-approval streak
is unreachable** because the LLM reviewers treat ADR-prose imperfections as
blockers and re-litigate every iteration. Directions to try (each needs a
live re-run to validate — tune one at a time):
1. **Calibrate `review_system`**: an ADR is APPROVED when it *factually and
   accurately records the decision the code embodies* (right sections, valid
   Nygard format, code refs resolve). Prose polish / nuance-completeness must
   NOT be blockers. This is the anti-Goodhart line: the gate is accuracy, not
   style.
2. **Treat low-confidence rejections as non-blocking in `streak_check`** (per
   the CLAUDE.md asymptote rule): add `|| input.confidence == 'low'` to the
   stop condition so a noisy nitpick doesn't reset the streak.
3. **Strengthen `prior_pushback` feedback** so a reviewer cannot re-raise a
   resolved item without new evidence (already wired — verify it's actually
   suppressing re-litigation in the prompt).
4. Consider bounding the per-run authoring batch (8 ADRs at once gives the
   reviewers a large surface to nitpick); authoring fewer per pass may
   converge more reliably.
Do **not** "fix" #6 by short-circuiting the review when the manifest is
aligned — that trades oscillation for a bypassed quality gate (Goodhart).
The fix is to make the reviewers converge, not to remove them.

Other operational notes: run Adry on a **clean tree** (finding #3); the
forfait token expires (~hours) so long runs may need a mid-run re-auth +
resume (finding #4); survey cost is the floor (~$3.58/run on pkg/git) since
survey_code always runs before the deterministic aligned-check.
