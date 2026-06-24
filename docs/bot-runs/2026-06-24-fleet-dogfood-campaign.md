# Fleet dogfood campaign — every catalog bot, one night (2026-06-24)

Goal: run **all** catalog bots on iterion itself; each must run to a terminal
state, produce quality, and not over-consume. Fix what doesn't (bot / engine /
claw). Repatriate verified work.

Setup: dedicated worktree `.works/fleet-campaign` off main `cde9640e6` (rebased
onto `0f1d8d670` at repatriation), fresh static binary, runs stored in the
operator-visible `.iterion` store. Every run: `--merge-into none`,
`post_to_board=false` where supported, per-bot `--max-cost-usd` caps, scoped vars.

## Scoreboard

| Bot | Persona | Result | Note |
|-----|---------|--------|------|
| review-pr | Revi | ✅ GREEN | 3 real findings, 0 false positives |
| docs-refresh | Doki | ✅ GREEN | caught + fixed real catalog drift |
| branch-improve-loop | Billy | ✅ GREEN | converged on small diff |
| test-coverage | Testy | ✅ GREEN | +156 lines real tests on pkg/log |
| feature-dev | Featurly | ✅ GREEN | shipped `bots list --format names` |
| feature-gap-fill | Fini | ✅ GREEN | shipped `validate --quiet` |
| rgaa-audit | Acci | ✅ GREEN* | *bot fix + scope; real RGAA report |
| adr-cartograph | Adry | ✅ GREEN* | *scoped; survey-runaway → engine fix |
| sec-audit-source | Seki | ✅ GREEN* | *via resume; deepsec is the runaway |
| whats-next | Nexie | ✅ GREEN | full 4-gate chain → 3 board tickets |
| adr-rechallenge | ReArchi | ✅ GREEN | keep decision, clean human-gate |
| whole-improve-loop | Willy | ✅ GREEN | converged, full test-suite verified |
| bmady | Bmady | ✅ GREEN | full BMAD, 5 gates driven |
| sec-audit-deps | Depsy | ⚠ AMBER | runs clean, 0 findings (known SCA scaffold) |
| evolve | Evoly | ❌ RED-known | gpt-5.5 forfait context overflow at aggregate_review |
| devbox-setup | Devy | ❌ RED | reproducible claude_code cold-start hang in sandbox |
| secured-renovacy | Renovacy | ✅ GREEN | safe-mode patch+minor → Phase-2 review → SBOM (e404438) |
| revi-converse | — | n/a | needs a live forge MR thread; not run offline |
| smoke | — | n/a | utility (no manifest); not a catalog bot |

**14 GREEN, 1 AMBER, 2 RED, 2 n/a** — all 18 catalog bots run.

## Fixes landed (campaign branch, verified, FF to main — NOT pushed)

1. **`fix(runtime): bound each node to remaining max_duration via a hard deadline`**
   — the high-value fix. `max_duration` was only checked at node *boundaries*, so a
   single long/hung node ran unbounded. This root-caused **three** separate run
   failures the same night: Seki's deepsec scanner ran 81m on a 90m budget, Adry's
   survey node ran 100m on a 50m budget, Acci's review stalled 43m after a stream
   timeout. Fix wraps each node's ctx with a deadline = remaining budget (kills the
   claude_code subprocess via CommandContext / claw stream), surfacing expiry as a
   resumable `BUDGET_EXCEEDED(duration)`. New test `TestNodeDeadlineFromDurationBudget`;
   full `pkg/runtime` suite green.
2. **`fix(rgaa-audit): inline env in node model fields`** — the engine resolves
   `${ENV:-default}` in a node `model:` field but **not** `{{vars.*}}`; rgaa was the
   only bot routing its model through a var, so `{{vars.detect_model}}` reached claw
   verbatim → "invalid spec". Inlined the env directly (matching the bot's own
   `max_duration` idiom); dropped the dead vars.
3. **`docs(bot-catalog): regenerate`** — the committed catalog was stale on two
   counts (rgaa vars + whole-improve-loop explore-mode vars from `dc22b626c`);
   docs-refresh and branch-improve-loop both kept rediscovering the same 1-line drift.
   Regenerated at source via `iterion bots regen-catalog`.

## Findings worth fixing next (not done tonight)

- **deepsec is the security-bot runaway**, not the generic toolchain. In the sec
  image, gitleaks/trivy/semgrep + gosec/bandit all ran clean & fast; only `deepsec`
  ran 81m and errored. CLAUDE.md's "deepsec ON is best path (generic broken)" is now
  **stale**. Recommend: `enable_deepsec` default **false**, and give deepsec a hard
  per-scanner timeout. (The new per-node deadline already bounds the runaway.)
- **whole-improve-loop no-source-scope gate** (Revi finding): in explore mode an
  empty `chunk_file_list` can never satisfy `files_reviewed > 0`, so a no-source
  scope never converges. Fix: add a `length(chunk_file_list)==0 ||` vacuous-true
  branch to the inlined guard in `engaged`/`clean_streak`/`stop` (streak_check, ~L1314).
- **Evoly gpt-5.5 forfait overflow**: `review_gpt` feeds the full investigation to a
  ChatGPT-forfait window that's too small on the FIRST request (compaction can't
  shrink an oversized initial prompt). Durable fix: bound `review_input`; env escape
  hatch `ITERION_EVOLVE_MODEL_GPT` exists.
- **Devy claude_code cold-start hang**: reproducible (failed alone *and* under load),
  even though Devy's sandbox config is identical to Seki/Depsy which work. Needs
  container-level debugging (exec in, check claude auth/first-token). Also:
  `ITERION_CLAUDE_CODE_STREAM_COLD_TIMEOUT` did **not** reach the sandboxed delegate
  (abort still at 90s) — the cold-timeout tunable doesn't propagate into the sandbox.
- **Depsy SCA scaffold** (native:3a81df64): heuristic scanner output still discarded
  → 0 findings. Honest coverage banner present; complete the scanner layer for a real
  dep audit.
- **Devy first-node non-resumable**: `detect_stack` is the entry node, so any failure
  is terminal (no checkpoint). A trivial deterministic pre-node would make it resumable.
- Minor: Nexie `emit_action` logged "Unknown skill: whats-next" (Skill tool lookup
  gap, non-fatal). Doki hand-edited the *generated* catalog file (content correct;
  doesn't know it's generated).

## Useful generated work (on storage branches — verify before cherry-picking)

- Featurly: `feat(bots): add "names" output format` — `38c64bc`
- Fini: `feat(validate): add -q/--quiet flag` — `5c44db4`
- Testy: `test(log): cover Level/IsEnabled/ParseLevel` (+156) — `3594b9b`
- Willy: `chore(improve): production-readiness pass` (pkg/log, +32) — `62ff660`
- Bmady: `validate` success-confirmation line — `055c2e4a`
- Adry: ADRs (scoped) — `9cb79ab`

## Cross-bot lessons

- The per-node duration deadline is the structural cure for "one node eats the whole
  budget" — it converted three hangs into clean resumable failures.
- Large-repo single-node surfaces (Adry survey, Acci 582-file review) over-consume;
  bots need scoping or chunking, and the deadline is the backstop.
- Human-gate bots are drivable unattended via `resume --answers-file`, but each gate's
  answer must match that node's **exact** output schema (Bmady: `approve_arch={approved}`
  vs `final_review={action:enum}`) or the resume fails NO_OUTGOING_EDGE.
