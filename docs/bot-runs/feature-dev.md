# Featurly — `feature-dev` run bilans

Autonomous end-to-end feature development: plan → act → `/simplify` →
prepare_commit → alternating Claude/GPT review-fix loop → commit, in an isolated
`worktree: auto`. See [bots/feature-dev/](../../bots/feature-dev/).

## 2026-06-24 — issue-comment → improvement-MR e2e on preprod (run 019ef703)
- Status: **partial** — the feature's TRIGGER half validated live on preprod (GitLab issue comment → `feature_dev` run launched); the bot run then failed on a pre-existing preprod infra gap, NOT the feature.
- Versions: bot feature-dev 0.1.0 (+ new `finalize_mr` tail) · iterion preprod `:edge` @`0f1d8d670` (this work) · webhook-launched on the cloud runner (sandbox).
- Method: shipped the issue-comment→improvement-MR feature (commit `0f1d8d670`); provisioned on preprod a gitlab webhook (`0b00720c`, wildcard) + a distinct project-194 bot PAT + `forge_token` bindings (feature-dev/whole-improve-loop/branch-improve-loop) on `devthejo/revi-playground` (194) + GitLab hook **#13**; posted `/featurly add a doc comment to fetch.go` on issue **!2**.
- Result: GitLab Note Hook → preprod parsed it as an **ISSUE note** (the new code — old code dropped issue notes as "not a merge-request note"), resolved `/featurly` → feature-dev, launched run `019ef703`. The run FAILED at sandbox start: `network proxy: kubernetes: ITERION_POD_IP env var is empty; the runner pod manifest must inject it via downward API (status.podIP)`. No MR (never reached `finalize_mr`).
- Value: HIGH for validation — proved the headline path (issue comment → deployed iterion → bot launch) end-to-end on real GitLab + preprod. The MR-generation half was proven separately by a local `finalize_mr` mechanics test against 194 (real MR opened + back-linked + cleaned up).
- Findings / misses:
  - **Trigger works on the deployed instance**: issue-note parse + `/featurly` route + bot launch all fire.
  - **Hand-created webhook needs `wildcard_bots`**: a webhook created via `POST /api/teams/{id}/webhooks` with explicit bot_ids has an EMPTY CommandMap, so `/featurly` filtered as "no command route" until I PATCHed `wildcard_bots=true` (then the live registry discovery resolves it). Documented, but a footgun — consider having the manual create endpoint compute the CommandMap from bot_ids (parity with the forge orchestrator's `buildCommandMap`).
- Engine/infra hardening (the real finding): **the iterion Helm chart's runner Deployment does not inject `ITERION_POD_IP` via the downward API (`status.podIP`)** → the k8s sandbox network proxy cannot start → EVERY sandboxed bot fails immediately on preprod (ovh-dev / chart `iterion` 0.14.0). Fix: add `env: [{name: ITERION_POD_IP, valueFrom: {fieldRef: {fieldPath: status.podIP}}}]` to the runner pod template. Blocks the deployed bot-run→MR e2e until fixed (a kubectl patch was correctly denied as a shared-infra change — needs operator consent).
- Lessons for next run: after fixing `ITERION_POD_IP` + the `wildcard_bots` webhook, the deployed e2e is **one `/featurly` comment from completing** — provisioning was left in place (webhook `0b00720c`, hook #13, bindings, bot PAT; issue !2). Loop-guard is satisfied (distinct bot PAT identity ≠ commenter).

## 2026-06-23 — Verified Action recovery ladder, ADR-044 (run 019ef38d)
- Status: **validated** — converged through the cross-family review loop to `done`; deliverable builds + all new tests green (verified independently in the worktree, anti-façade).
- Versions: bot feature-dev 0.1.0 · iterion fresh static (campaign HEAD) · `claude_code`/opus · `worktree: auto` · `--merge-into none`.
- Method: one large `feature_prompt` asking Featurly to implement the **entire** ADR-044 "Verified Action" synthesis (goal+recipe+postcondition+policy quad; idempotent-skip → recipe → self-repair → agent-recovery escalation; postcondition-as-truth; gates stay deterministic) one-shot. Run was cap-interrupted at 08:58 (Anthropic session limit) at `reviewer_claude`, resumed cleanly from checkpoint on a switched account → finished.
- Result: branch `iterion/run/019ef38d-745b…` @ `79f9111ed`, 1592 insertions / 22 files: DSL (parser/AST/jsonenc/IR + `validate_verified_action.go`/unparse/EBNF), runtime (`executor_verified_action.go` 342L ladder + `executor_tool.go` wiring + engine.go + new event), unit + e2e tests, docs (ADR-044, DSL quickref, CLAUDE.md), and a demo application on `secured-renovacy`'s commit node. `go build ./...` + `go test ./pkg/backend/model ./pkg/dsl/{ir,parser,unparse}` green.
- Value: HIGH — delivered a whole engine subsystem (the action-node robustness pattern) in a single supervised run, on the operator's explicit directive. NOT yet merged to main (overlaps the 5 hand-fixes to Renovacy's commit node landed the same day; needs review of `executor_verified_action.go` + overlap resolution).
- Engine hardening: the run itself is the proof-of-need for ADR-044 — see the 5 brittle commit-node failures on `secured-renovacy` the same day ([secured-renovacy.md](secured-renovacy.md)).
- Lessons for next run: feature-dev handles a large multi-layer engine feature well when the prompt carries the full design + an explicit anti-façade done-criterion + a reference to the ADR; resume-from-checkpoint survived a mid-run provider cap with zero lost work.

## 2026-06-17 — ADR-028 Steps 2-4 dispatcher I/O offload (runs 019ed4cd, 019ed4eb, 019ed51d)
- Status: 2 validated+converged (Steps 2, 3) · 1 implemented+validated+manually-repatriated (Step 4 — bot review loop blocked by a runtime stall, not the code)
- Versions: bot feature-dev 0.1.0 · iterion run-binary fresh static `fe132645` · dispatched via the **dispatcher** (own `iterion dispatch` daemon on the operator's repaired config, `--no-server`, sandbox `iterion-sandbox-full:edge`, `worktree: auto`). Each step: isolated ticket with an anti-façade done-criterion, reviewed + race-verified + repatriated before the next.
- Result:
  - **Step 2** (ListCandidates off-actor) — converged ~27 min, `77a2cb80`, FF to main. `launchDiscovery`→`cmdCandidates`, single-flight `discoveryInFlight`, `postCmd` shutdown-safe choke point.
  - **Step 3** (finishRun tracker HTTP off-actor) — converged, `a72d40f7`, FF. `finishPlan` value-copy; transition-FIRST/Release-LAST to close the re-dispatch window; optimistic-retry-as-guard for the give-up HTTP window (`cmdDropRetry`).
  - **Step 4** (post-claim UpdateState + workspaces.Create off-actor; Claim stays atomic — the reduced/safe variant, chosen over full optimistic-claim) — implemented + build/vet/gofmt clean + full dispatcher race suite green + 3 anti-façade tests pass, BUT the bot's own review loop could NOT converge: `fix_gpt` (sandboxed gpt-5.5 via claw) repeatedly hit "context canceled" at the dispatcher's 10-min **stall timeout**, looping retry→re-dispatch→stall. I reviewed the uncommitted worktree directly (max rigor), confirmed correctness, and **manually repatriated** (`9b3bd3bd` → cherry-pick `70b3d4ed`, auto-merged clean over the operator's parallel commands.go bug-sweep).
- Value: high — ADR-028 Steps 2-4 land; discovery, finishRun, and post-claim dispatch I/O are now all off the actor goroutine (only `RefreshStates` remains, deliberately deferred).
- Findings / quality: exemplary anti-façade across all three. The Step-4 standout: it kept Claim atomic, allocated the slot post-claim (`setupPending=true`), and guarded **both** reapers (`refreshRunningStates` + `reconcileStalled`) against the setup window — correctly identifying that the off-actor `UpdateState` makes the tracker read RunningState before the entry records `TransitionedFromState`, which would otherwise self-cancel the run.
- Engine hardening (the real finding): **`fix_gpt`/reviewer-fix on sandboxed `gpt-5.5` (claw) hangs >10 min → trips the dispatcher's 10-min stall timeout → cancel → retry loop**, blocking review-loop convergence on a perfectly good change. Runtime issue (sandboxed claw streaming / context on a large review-loop context), not the bot. Relates to the known sandboxed-claw streaming + gpt-5.5-forfait-context work. Worth a ticket. Secondary: the run-status monitor false-terminals on a transient cancel→auto-resume — key on issue-state, not run-status.
- Lessons for next run: a review loop stuck on a RUNTIME stall ≠ bad code — validate the worktree directly (build + `-race` + manual review) and repatriate rather than re-dispatching into the same stall. For the riskiest step, the reduced variant (Claim-atomic, offload post-claim) avoids the reserved-before-claim state entirely and was the right call.

## 2026-06-15 — ADR-028 + Step 1 lock-free dispatcher Snapshot (run 019ecafa)
- Status: validated
- Versions: bot feature-dev 0.1.0 · iterion run-binary fresh static build of main `8477a067` (≈ HEAD)
- Method: dispatched via the **dispatcher** (own `iterion dispatch` CLI, `--no-server`, fresh static binary so delegated subprocesses + sandbox mount use current code; workspace store `.iterion`). claude_code (Opus 4.8) plan/act/`/simplify`/reviewer_claude; claw GPT reviewer_gpt; `sandbox: iterion-sandbox-full:edge`, `worktree: auto`. Ticket = ADR-028 body (decomposed I/O-offload roadmap) with an anti-façade Step-1 scope.
- Result: **converged in one review round** — plan → act → simplify → reviewer_claude → streak_check → reviewer_gpt → prepare_commit → commit_changes → done. `finished`, ~16 min, ~56k tokens. 1 commit `89dd2f57` on branch `iterion/run/aurora-hunt-prismpunk-01af`; **repatriated to main by FF** (clean — main was ancestor); issue `efb9022d` → done.
- Value: high. Produced `docs/adr/028-dispatcher-actor-io-offload.md` (records the tracker-as-claim-authority insight + per-issue state-machine direction + incremental sequence + rejected alternatives) AND implemented Step 1: `Snapshot()` is now lock-free (`atomic.Pointer[Snapshot]`, published by the actor in `fireSnapshot`, seeded at construction), so dashboard reads never wait on the actor's in-flight tracker I/O. Removed the now-dead `cmdSnapshot`.
- Findings / quality: exemplary anti-façade output. (1) It **refused** the nil-fallback to `buildSnapshot()` with the correct reasoning that it "would read c.state off the actor goroutine — the very race this read path removes" — i.e. it understood the invariant, didn't just add a field. (2) Scoped strictly to the read path; no out-of-scope dispatch/claim/finishRun changes. (3) The test (`TestSnapshotLockFreeWhileActorBlocked`) gates `fakeTracker.ListCandidates` on a channel, waits until the actor is *provably* parked inside it, then asserts `Snapshot()` returns < 500ms with real state — genuinely proves the property, not its shape. Independently re-verified: build/vet OK, race-clean 3×.
- Engine hardening: none needed from the run. The dogfood surfaced an **environment bug**: the operator's `.iterion/dispatcher/dispatcher.json` routed every bot to stale `examples/<bot>` paths (bots had moved to `bots/<bot>`), so `iterion dispatch` refused to start (`stat examples/branch_improve_loop: no such file`) and the studio's *embedded* dispatcher had silently degraded to an inert stub (`slots.global_max=0`, `tracker=""`) — enabled but never dispatching. Config validation failed loudly for the CLI (good); the studio swallowed it into a stub (worth surfacing in the dashboard). Worked around with a minimal repaired config (`feature-dev → bots/feature-dev`); operator's original backed up to `/tmp/operator-dispatcher.json.bak`.
- Lessons for next run: feature-dev handles a doc+code+test feature cleanly when the ticket carries a concrete, anti-façade *done criterion* (here: "a test proving the read returns while the actor is provably blocked; adding the field is not sufficient"). Keep the dispatcher config current when bots move dirs (`examples/*`→`bots/*`), and consider a startup banner when the embedded dispatcher degrades to a stub.

## 2026-06-13 — sandbox-doctor static-binary check (runs 019ec149, **019ec180**)

> **Update — fix applied + validated (run 019ec180).** Taught `act`/`fix` to
> `git -C <workspace_dir> add -A` after editing (commit `44d34c9d`), so new files
> are tracked and visible to the reviewers' `git diff HEAD`. Re-running the SAME
> feature_prompt: Featurly **converged and committed** (`finished`, **$2.85 / 247
> steps** vs the looping `$4.95 / 507 / cancelled`), shipping commit `439d1116`
> on `iterion/run/opal-flash-mothbeam-80d7` — `pkg/cli/sandbox.go` (+106, the
> doctor static/dynamic ELF check + WARNING), a **tracked** test, AND
> `docs/adr/019-sandbox-doctor-static-binary-check.md`. The new test being in the
> commit is the direct proof the untracked-files bug is fixed. Feature pending
> integration to main (after the parallel Depsy run, to avoid a watchexec restart).

- Status (original run 019ec149): **failed to converge — implementation correct,
  review loop broken for new-file features → FIXED + validated (see update above).**
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
