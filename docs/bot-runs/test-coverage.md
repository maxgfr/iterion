[← Bot runs](README.md)

# test-coverage (Testy) — run bilans

Universal test-coverage augmentation bot. Plans missing tests for a target
area, writes them with the repo's own framework, proves they pass with a
deterministic gate, then runs the cross-family (Claude + GPT) anti-façade
review loop to a `test:` commit. Modelled on feature-dev + whole-improve-loop's
verify gate; stack knowledge lives in `skills/` (test-coverage, verify-tests,
test-types). Anti-façade is the design center: the metric is meaningful tests
that catch a real regression, NOT coverage %.

---

## 2026-06-23 — first dogfood, pkg/log unit (runs 019ef4fa + 019ef505)
- Status: **validated** — full cross-family convergence to a clean `test:`
  commit. (Surfaced + fixed one engine bug along the way; the GPT half was
  briefly blocked by an exhausted OpenAI forfait, resolved by switching Codex
  account + `iterion resume`.)
- Versions: bot 0.1.0 · iterion (working tree on `main` @ e2cd45c + this work)
- Method: `iterion run bots/test-coverage/main.bot --var target=pkg/log --var
  test_unit=true --merge-into none`; backends claude_code (opus-4-8) for
  plan/act/simplify/reviewer_claude/prepare_commit, claw `openai/gpt-5.5` for
  reviewer_gpt; sandbox `iterion-sandbox-full:edge`, `worktree: auto`,
  network open.
- Result: **converged to `done`.** plan → act → simplify → **verify gate PASSED
  first try** (`passed=true, new_test_code=true`, 537 ms, no repair loop) →
  reviewer_claude **approved (high, 0 blockers, 7 areas)** → [forfait 429,
  resumed] → reviewer_gpt **approved (high, 0 blockers, family=gpt)** →
  streak_check **stop** (cross-family double approval) → prepare_commit picked
  **only `pkg/log/log_test.go`** (correctly excluded an unrelated catalog
  regen line) → commit_changes → done. Commit **aac2ab2** on storage branch
  `iterion/run/019ef505…` (`--merge-into none`, main untouched); message
  `test(log): cover Truncate rune-boundary and writer levels` (+386 lines).
  Cost ≈ **$2.9** total, ~13 min wall across the initial run + resume.
- Value: **high.** Produced 12 meaningful unit tests for `pkg/log`
  (`pkg/log/log_test.go` +399 lines) — 51 assertions, 38 table cases, targeting
  edge/error paths (nil-receiver safety, JSON marshal-failure drops the line,
  UTF-8-safe byte cut, level resolution precedence, emoji gating). **Coverage
  58.1% → 99.2%.** Repatriated to the main tree and independently verified
  (build + `go test` + vet + gofmt all green, 99.2%).
- Findings / anti-façade behaviour:
  - The agent **self-mutation-checked** during `act`: "disabling the
    `safeByteCut` walk-back made TestSafeByteCut/TestTruncate/TestBlockPreview
    fail … restoring it passed. Proves the tests catch a real regression." The
    skill's mutation-test doctrine reached the implementer concretely.
  - reviewer_claude applied the mutation lens and approved with high confidence
    + zero blockers — no façades to catch (the deterministic gate + plan kept
    quality high upstream).
  - The deterministic `verify_run_tests` gate passed first try
    (`passed=true, new_test_code=true`, 537 ms) — no repair loop needed.
  - `extra_test_kinds`/checkbox substitution rendered correctly in prompts
    (`unit: true …`); empty selection would have let the bot choose.
- Engine hardening:
  - **claude_code accepted "API Error: 5xx" as a successful result** (run 1,
    019ef4fa): an Anthropic 529 overload was rendered by the CLI as the `plan`
    node's *result text*, so the node "succeeded" with a non-plan and `act`
    inherited a poisoned session and hung. Fixed: `isTransientAPIErrorResult`
    in [pkg/backend/delegate/claude_code.go](../../pkg/backend/delegate/claude_code.go)
    re-types a transient API-error-as-result (429/5xx/529/connectivity, short +
    `API Error:` prefix) to `ErrTransient` so the executor retries; 4xx
    client/auth errors still surface. 21-case unit test
    ([claude_code_apierror_test.go](../../pkg/backend/delegate/claude_code_apierror_test.go)).
  - **Skill gap (fixed inline):** in the sandbox, `devbox run` fails
    (`~/.cache/devbox` unwritable); the agent adapted to plain `go test`, and
    `skills/verify-tests.md` now documents the `XDG_CACHE_HOME=/tmp` retry →
    direct-toolchain fallback.
  - Minor: the engine auto-regenerates the bot catalog into the worktree at run
    start, adding a 1-line unrelated diff (sec-audit-deps `scope_notes`).
    Harmless — `prepare_commit` excludes non-test files by design.
- Lessons for next run:
  - reviewer_gpt/fix_gpt require a working non-Anthropic backend. The ChatGPT
    **forfait** quota (`429 usage limit reached`) is a hard cap a retry can't
    clear — switching the Codex account (or setting `OPENAI_API_KEY`) + `iterion
    resume --run-id … --file … --force` picks up the new creds (the resumed
    sandbox re-mounts `~/.codex`) and closes the loop from the checkpoint.
  - On **resume**, the `prepare_commit` session-fork is dropped ("parent session
    has no recorded provider fingerprint") and it starts fresh — harmless, it
    re-reads `git diff HEAD` to build the commit. Expected resume-path behaviour.
  - `iterion resume` has **no `--timeout` flag** (unlike `run`).
  - Bot is validated end-to-end. Next: a multi-type run (integration/e2e) on a
    repo with a real e2e harness, and a no-checkbox "bot chooses" run, to
    exercise the type-selection + scope-auto paths.
