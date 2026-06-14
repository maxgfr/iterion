# adr-cartograph (Adry)

Read-only ADR cartographer + completeness auditor.

## What it does

Adry walks the code as currently implemented and produces committable
**Architecture Decision Records** in `docs/adr/` (Nygard format). Every
ADR Adry writes is a **constat** — a record of the decision the code
embodies, with the trade-offs it implies and the alternatives that were
not taken — so a future maintainer can re-challenge it later when the
constraints change.

Alongside the ADR pass, Adry produces a **completeness audit** for
in-flight features: what is fully implemented, what is stubbed, what is
TODO-marked, what was wired but never tested. The audit lives in the
generated ADRs themselves (under a "Consequences / Known gaps" section)
and, optionally, as `type:feature-gap` issues on the native board.

## Idempotency guarantee

Adry is **reasonably idempotent**. If the ADRs already match the code and
nothing drifted, a re-run does (almost) nothing — no new ADR, no commit,
no board churn.

The levers:

- `scan_adrs` cross-references the inter-run sha-cache at
  `.adr-cartograph-cache.json` (repo-root dotfile, gitignorable). When
  every ADR's content sha AND its cited code-file shas match the prior
  run, the ADR is `pre_verified` and the survey/build_manifest passes
  skip it.
- `detect_changes` is a deterministic `git status --porcelain` guard
  between the converged review and the commit phase: a review-only
  convergence (no new/edited markdown) bypasses `prepare_commit` +
  `commit_changes` and goes straight to the audit-cache refresh + done.
- Handoff issues are filed **inside** `prepare_commit`, so a no-op
  re-run never spams the board.

## Read-only on code

Adry **never** edits source code. Its fixer agents (`fix_claude`,
`fix_gpt`) are scoped to `docs/adr/`, and the `enforce_fix_scope`
deterministic tool reverts (`git checkout --`) any edit whose path does
not start with `{{vars.adr_dir}}/`. The only outputs of an Adry run that
touch the working tree are `.md` files under that directory.

## Handoff to sibling bots

Adry can file **optional** backlog issues on the native board (claude_code
runtime opens `mcp__iterion_board__*` tools when the `prepare_commit`
node's `capabilities:` list includes `board.create` + `board.assign`):

- **`type:adr-rechallenge`** — when an ADR is older than
  `--var rechallenge_after_days=N` (`0` = disabled, the default) or when
  the recorded "consequence triggered" condition has fired. Routed to the
  `adr-rechallenge` bot via `set_bot`.
- **`type:feature-gap`** — for each in-flight feature whose completeness
  gap is severity `medium` or `high`. Routed to the `feature-gap-fill`
  bot via `set_bot`; the gap spec rides on `fields.bot_args`.

Both follow whats-next's discipline: `list_labels` first (board.read)
before assigning labels, `set_bot` (not `assign_issue`) for bot routing,
the `source:adr-cartograph` label so the issues are traceable back.

## Key vars

| var | default | meaning |
|---|---|---|
| `workspace_dir` | `${PROJECT_DIR}` | repo root (the worktree under sandbox). |
| `adr_dir` | `docs/adr` | universal Nygard convention. Override per-repo if needed. |
| `audit_cache_path` | `.adr-cartograph-cache.json` | repo-root dotfile — gitignorable. NOT `.iterion/`. |
| `code_scope_globs` | `""` | empty = whole workspace minus `excluded_dirs`. |
| `excluded_dirs` | `.iterion,.works,.claude,vendor,node_modules,.git,dist,build,out` | hard-skip dirs. |
| `diff_since` | `""` | optional incremental hint (e.g. `--var diff_since=main`). |
| `max_review_iterations` | `10` | bounded review loop. |
| `max_recovery_iterations` | `15` | bounded fix loop. |
| `coverage_target_pct` | `80` | streak gate threshold. |
| `rechallenge_after_days` | `0` | `0` = never invite re-challenge; `90` = age out after 3 months. |
| `scope_notes` | `""` | operator's attention pin (dispatched from issue body). |
| `bundle_self_path` | `""` | set to `bots/adr-cartograph` when Adry runs against the iterion repo. |
| `issue_id` | `""` | dispatcher-attached issue id (no-op when run manually). |

## Workflow shape

`scan_adrs` → `survey_code` → `build_manifest` → alternating
`reviewer_claude` (claude_code/opus) / `reviewer_gpt` (claw/gpt-5.5) →
`streak_check` (cross-family double-approval + coverage gate) →
`detect_changes` → `prepare_commit` (drafts message, files handoff
issues) → `commit_changes` → `mark_issue_for_review` →
`update_cache` → `done`.

Fix path: `fix_claude` / `fix_gpt` (writes ADR markdown ONLY) →
`enforce_fix_scope` (reverts non-`docs/adr/` edits, bounded
`recovery_loop`) → `build_manifest` (re-run so reviewers see the fresh
state).

Loop-exhaustion fallthroughs route to `fail` (NOT `done`) so a
deterministic non-convergence is visible.

## Skills shipped

| skill | purpose |
|---|---|
| `adry.md` | operating playbook — mission, immutable rules, idempotency, handoff. |
| `adr-format.md` | the EXACT Nygard format used in `docs/adr/` (filename, front-matter, sections). |
| `adr-scope-detection.md` | heuristics for locating where ADRs live in a repo. |
| `decision-vs-mechanic.md` | what is ADR-worthy vs what is a mechanical refactor. |
| `completeness-taxonomy.md` | enum-locked kinds of feature gap Adry recognises. |
