# `secured-renovacy.iter` — design notes & observations

A running log of what we learn iterating on this recipe. Append-only when possible; rewrite a section when a previous claim is invalidated by a new experiment.

## Architecture in one paragraph

Phase 0 (`detect_stack` + `capture_start_sha`) identifies the project's package manager and the upgrade base SHA. Phase 1 is a per-package loop driven by an agentic chain: `discover_outdated → select_candidate → security_audit → changelog_review → upgrade → install → align_code → validate_upgrade → (fix_loop)* → prepare_commit → commit_changes → (loop)`. Phase 2 alternates Claude and GPT reviewers over the cumulative diff with auto-fixes between rounds. Every node that touches a package manager is an agent; only `select_candidate`, `join_files`, `mark_failed_and_continue`, `streak_check`, `capture_start_sha`, and the commit tools are deterministic.

## Design principles (in priority order)

1. **Reliability beats cheapness.** Token cost is acceptable when it buys reproducibility. An LLM call that lands the right command on every ecosystem beats a deterministic shell pipeline that works on three of them and silently breaks on the fourth.
2. **No fallbacks.** When a layer looks fragile, fix the root cause (rebuild the binary, tighten the prompt, restructure the node) instead of catching the symptom downstream. The pre-agentification `commit_changes` printed `success:true` regardless of git's exit code; the agentified upgrade absorbed package-shape variability by chance. Both were "fallbacks" that hid real failures.
3. **Structure with moderation.** Typed fields and structured output are useful when a NON-agent consumer reads them (a deterministic tool, the UI, a metric). Multiplying typed fields between LLMs creates friction: every upstream has to produce exactly, every downstream parses exactly, and reality (vendored shims, partial info, edge cases) becomes a contract violation.
4. **Re-use sessions when continuity helps.** Modern Claude models have generous context windows; the cost of `session: fresh` is paying again to re-establish project context that a `session: inherit` would inherit for free (and from prompt cache). Use `fresh` only when the prior context is irrelevant (the first detect_stack pass, the cross-family GPT reviewer).
5. **Loose `notes` over multiple structured "shim/config" fields.** `detect_stack` writes a single `notes: string` for everything the downstream agents need to know about toolchain shims, registry auth, hooks, peculiarities. Promoting these to typed fields (`command_prefix`, `registry_config`, `manager_version`) was tried and reverted — the rigidity wasn't worth the marginal clarity.

## Why each node is agent vs tool

| Node | Kind | Why |
| --- | --- | --- |
| detect_stack | agent | Pure inspection / classification — LLM excels at "what does this repo look like". |
| capture_start_sha | tool | One git command, one JSON output. Pure determinism. |
| discover_outdated | agent | Each ecosystem's "outdated list" requires distinct invocations + JSON shape transforms. Sonnet handles the per-ecosystem branching trivially; a deterministic jq pipeline used to silently fail on yarn berry. |
| select_candidate | tool (jq) | Filter / sort / pick-first is pure data. Earlier Sonnet version absorbed shape coercions by luck (the structured-output formatter kept wrapping the package array as `{list: [...]}` so the agent had to keep guessing) — non-reproducible. The jq tool normalises the three observed input shapes and applies the policy deterministically. |
| security_audit | agent | Native auditor output formats differ wildly per ecosystem; verdict requires reasoning about which advisories actually affect the target version. The prior `grep -qiE 'high\|critical'` shell pipeline produced silent false positives and false negatives. |
| changelog_review | agent | Reading release notes and inferring breaking impact is exactly the LLM's strength. |
| upgrade | agent | Toolchain reality (corepack shim, vendored `.yarn/releases/yarn-X.Y.Z.cjs`, `python -m poetry`, `cargo update --precise`) varies per repo. Deterministic command authoring used to die when the binary wasn't on PATH. |
| install | agent | Same toolchain story as upgrade. |
| align_code | agent | Applies LLM-authored alignment steps; obviously LLM-shaped. |
| validate_upgrade | agent | Picks build/test/lint commands appropriate to the stack and reads their output for blockers. |
| fix_after_upgrade | agent | LLM-driven fix application bounded by `fix_loop`. |
| prepare_commit | agent | Drafts commit messages following the repo's existing convention. |
| revert_changes | agent | Same toolchain story as install (needs to reinstall after git checkout). |
| commit_changes | tool | git add / commit / amend pipeline — deterministic; needs `set -euo pipefail` to actually fail on git errors (previous version always reported success). |
| reviewer_claude / reviewer_gpt / fix_claude / fix_gpt | agent | Phase 2 cross-family review. |
| review_commit_auto | tool | Same shape as commit_changes. |

## Observations from iteration runs

### 2026-05-11 — agentification + codex review pass 1

**Worked first try.**
- detect_stack on yarn berry: notes correctly flagged the vendored release shim. ✓
- discover_outdated on yarn berry: produced 58-package list by parsing yarn.lock + querying npm registry. The agent built a node script inline since `yarn outdated` doesn't exist in berry. ✓
- security_audit verdict on `@opentelemetry/api 1.9.1`: parsed yarn audit + cross-referenced GitHub Advisory DB when osv-scanner was unavailable in the sandbox. ✓
- upgrade on yarn berry: invoked `node .yarn/releases/yarn-3.6.1.cjs up --exact` after testing `which yarn`. ✓
- install + align_code + validate_upgrade + prepare_commit all passed. ✓

**Failed first try, fixed in pass 1.**
- `commit_changes` reported `success:true` while `git commit` died on missing `Author identity` — the trailing `printf '{"success":true,...}'` always fired. Fixed: `set -euo pipefail` + explicit exit checks on every git call + git identity env in sandbox spec.
- `capture_start_sha` emitted bare stdout (the SHA) which iterion auto-wraps as `{result:...}`; Phase 2's `outputs.capture_start_sha.sha` therefore resolved to empty, making the cumulative-diff base wrong. Fixed: tool now emits explicit `{"sha":"..."}` JSON.

**Failed first try (recipe), fixed in pass 2.**
- `select_candidate` as a Sonnet agent absorbed whatever `packages` shape the formatter produced — sometimes object-keyed-by-name, sometimes `{list: [...]}` wrapper, sometimes a flat array. Per-run divergence. Fixed: de-agentified to a deterministic jq tool that normalises all three shapes.
- Initial jq tool wrote temp files under `/tmp` via `mktemp -d` — the sandbox image rejected this with "Directory nonexistent". Fixed: write temp files under `workspace_dir` (the bind-mounted writable volume).

**Engine bug found in pass 2.**
- `fix_loop(5)` was being consumed across the WHOLE run instead of per-package. Once any single package exhausted fix_loop, every subsequent package got zero retry budget. Fixed in the iterion runtime (not the recipe): `Loop.Body` is now computed at compile time as the non-loop-edge cycle, and `selectEdgeRS` resets a loop's counter when a non-loop edge re-enters its body from outside. Each package_loop iteration now starts fix_loop fresh.

**Reverted (lesson: less structure is more).**
- `manager_version`, `command_prefix`, `registry_config` were promoted out of `notes` into typed fields. Reverted: forcing structured exchange between agents adds rigidity without proportional benefit. Downstream agents handle toolchain reality from `notes` fine.

## Open critiques to address next

From the 2026-05-11 codex GPT-5.5 review (see `memory/project_secured_renovacy_codex_review.md`):

- **Multi-ecosystem (codex #3):** the recipe assumes a single `pkg_manager`. Real repos commonly have Maven + Docker tooling, Go + JS frontend, Python + JS build glue. Solution direction: `stack_profile.ecosystems: [...]` (one entry per detected stack); Phase 1 either fans out per ecosystem or iterates serially.
- **Multi-workspace identity (codex #4):** `discover_outdated`'s package map keys by name only. Same package in two workspaces with different ranges collapses. Solution direction: include `workspace` and `manifest_path` in the package record; `select_candidate` and `upgrade` carry workspace through.
- **Stale snapshot (codex #5):** the initial `discover_outdated` snapshot is reused for every loop iteration. After a commit, target versions can shift (transitive constraints relax). Solution direction: re-run discover after each commit or maintain a per-package ledger of "fresh as of commit X".
- **Phase 2 silent finish (codex #13):** `review_loop` exhaustion routes to `done` instead of escalating unresolved blockers. Solution direction: add a `fail_with_blockers` node + route loop-exhausted edges there.
- **Reproducibility pin (codex #14):** sandbox image is `:latest`; claude-code is installed `@latest` in post_create. Operators upgrading to production should pin to digests / versions.

## Open invariants the runtime enforces

- **Per-entry loop budget** (engine change in pass 2): once a non-loop edge re-enters a loop's body from outside, that loop's counter resets. Means `fix_loop(N)` is N retries per package, not N total.
- **Reasoning-effort coercion** (engine change in pass 1): `reasoning_effort: max` is clamped to the model's highest supported tier (`high` on OpenAI, `max` on Anthropic 4.7+). Recipes can ask for `max` everywhere without worrying about provider rejection.
- **Pass 2 sandbox-aware structured output** (engine change in pass 1): formatOutput now routes through the sandbox command builder. No more empty Pass 2 results from "No conversation found".
