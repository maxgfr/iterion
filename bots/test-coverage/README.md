# test-coverage (Testy)

Autonomous test-coverage augmentation. Plans which tests are missing for a
target area, writes them with the repo's OWN test framework, proves they pass
with a deterministic gate, then runs the alternating Claude/GPT review-fix loop
until two consecutive cross-family approvals — rejecting façade tests at every
step.

**Anti-façade by design:** the success metric is meaningful tests that would
catch a real regression, never coverage percentage. See
[skills/test-coverage.md](skills/test-coverage.md) for the doctrine.

## Inputs

| Var | Required | Default | Description |
|---|---|---|---|
| `target` | no | `""` | Path / package / area / free description to cover. Empty ⇒ Testy picks the lowest-coverage / most-critical / recently-changed code. |
| `test_unit` | no | `false` | ☐ Add unit tests. |
| `test_integration` | no | `false` | ☐ Add integration tests. |
| `test_e2e` | no | `false` | ☐ Add end-to-end tests. |
| `extra_test_kinds` | no | `""` | Free-text "other" kinds (property-based, contract, snapshot, smoke, performance…). |
| `workspace_dir` | no | `${PROJECT_DIR}` | Workspace root (resolves to the run worktree under `worktree: auto` — do not override). |

When **no** test type is checked and `extra_test_kinds` is empty (the default),
Testy chooses the types that fit the code and the repo's conventions.

## Pipeline

1. `plan` — Claude Code, read-only → detect stack, find coverage gaps, choose
   test types, structured test plan
2. `act` — Claude Code, session-inherit → write the tests, make them pass, write
   the verify script, `git add -A`
3. `simplify` — Claude Code, native `/simplify` on the new tests
4. `verify_tests` — **deterministic gate** (no LLM): re-runs the repo's own test
   suite and confirms genuinely-new test code was added
5. `alt → reviewer_claude / reviewer_gpt → streak_check → fixers` — cross-family
   anti-façade review loop; stops on double approval
6. `prepare_commit → commit_changes` — semantic `test:` commit

## Run

```bash
iterion run bots/test-coverage/main.bot \
  --var target='pkg/log' --var test_unit=true
```

Stack-agnostic: how to detect the runner, where tests live, and how to write
each test type lives in [skills/](skills/) — adding a language needs no DSL edit.
See [main.bot](main.bot) for the full DSL.
