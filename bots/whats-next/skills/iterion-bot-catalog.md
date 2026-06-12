---
name: iterion-bot-catalog
description: Catalog of iterion bots — pick a bot name for each roadmap_item.assignee. The stock dispatcher routes by assignee through assignee_workflows.
---

# Iterion Bot Catalog — for whats-next.bot's `propose_roadmap`, `revise_roadmap`, and `emit_action`

<!-- This file is the HAND-AUTHORED TEMPLATE for the bot catalog. The
     persona table + per-bot reference cards between the GENERATED markers
     below are produced from each bot's manifest.yaml by
     botregistry.RegenerateWhatsNextCatalog (run at whats-next start and
     on every studio bot-metadata save). Do NOT hand-edit that region —
     edit the bots' manifest.yaml instead (display_name / description /
     when_to_use / triggers / enabled), or toggle a bot in the studio
     Catalog manager. Everything OUTSIDE the markers is editorial routing
     reasoning you maintain by hand. This template lives at the bundle
     ROOT (not skills/) so it is never mirrored as a skill; the generated
     copy Nexie actually reads is skills/iterion-bot-catalog.md. -->

Consumed by three phases:

1. **`propose_roadmap` / `revise_roadmap`** — pick the right
   bot name for each `roadmap_item.assignee`. Leave it `""`
   when no existing bot fits.
2. **`emit_action`** — validate every assignee against the
   catalog before creating issues. Unrecognised assignees get
   stripped to `""` and the issue is labelled
   `needs-manual-triage`.

**Trust check first**: this catalog enumerates bots discovered
in the workspace. If the workspace ships no bots (none of the
cards below resolve), all assignees should be `""` and all issues
will be `needs-manual-triage`.

## The pivot: kanban-driven, not shell-driven

whats-next.bot no longer shells out `iterion run <bot>`. Instead
every roadmap item becomes a kanban issue on the native board at
`<workspace>/.iterion/dispatcher/`, and a **dispatcher** dispatches
them. The dispatcher is wired via `iterion dispatch <config.yaml>`.

**How the stock dispatcher picks a workflow per issue today**:
workflow routing is done by the runner built at `iterion dispatch`
startup, not by switching workflows inside a running `EngineRunner`:

1. **`assignee_workflows:` map** — when the issue's `assignee`
   has an entry in the dispatcher YAML's `assignee_workflows:`
   map, `RoutingRunner` selects the precompiled runner for that
   workflow. See
   [docs/dispatcher.md §Routing by issue assignee](../../../docs/dispatcher.md).
2. **registry fallback** — when the assignee has no
   `assignee_workflows:` entry, the dispatcher resolves it against
   the discovered bot catalog (any enabled bot is routable by its
   technical name) and runs that bot's workflow.
3. **`workflow:` default** — the precompiled global fallback when
   the assignee is empty or unresolvable.

Native issues also have typed `Bot` / `BotArgs` fields. `BotArgs`
merges over rendered dispatch vars and is usable today.

`assignee_dispatch:` (when present) replaces `dispatch.vars`
wholesale per assignee; per-ticket `BotArgs` then merges on top
key-by-key (see the issue-creation section below).

whats-next records the assignee on every issue so operators can drive
routing by setting `--assignee` and mapping it through
`assignee_workflows:` (or relying on the registry fallback).

## Decision tree — pick `assignee` per roadmap item

Walk top-to-bottom; first match wins.

| If the work sounds like… | → `assignee` |
|---|---|
| "implement feature X", "add capability", "build the thing" | `feature-dev` |
| "build a new bot for Y" / "create a workflow that does Y" — the catalogue lacks a fit and we need to author one | `feature-dev` (with `feature_prompt` pointing at the new `.bot` file to create) |
| "review the whole codebase", "audit production-readiness", "find bugs anywhere" | `whole-improve-loop` |
| "focus on axis X" (observability / perf / DX / refactoring) ACROSS the codebase — improvement loop, not detection | `whole-improve-loop` (with `--var improvement_prompt=…`) |
| "review this branch", "review the PR", "fix the diff against main" — review AND fix AND commit | `branch-improve-loop` |
| "review this PR / branch and just REPORT the issues" — read-only review, posts findings to the board, does NOT fix or commit | `review-pr` |
| "upgrade dependencies", "patch CVEs", "bump versions", "renovate" — MUTATING (writes package.json / go.mod / lockfiles) | `secured-renovacy` |
| "audit the docs", "find code↔doc drift", "doc/code alignment", "fix outdated README/CLAUDE.md" | `docs-refresh` |
| "audit the source for vulns", "find injection / SSRF / IDOR / secrets", "OWASP source scan" — DETECTION (writes findings, not fixes) | `sec-audit-source` |
| "audit dependencies for malware / typosquats / install hooks", "supply-chain check", "post-`npm install` triage" — DETECTION across installed deps | `sec-audit-deps` |
| architectural choice, hiring, prioritisation meeting, alignment | `""` |
| operator is vague or it's cross-cutting | `""` |
| long-term theme (a quarter+ horizon) | usually `""` |

When in doubt, prefer `""` and let the operator triage manually
in the board UI. An empty assignee is honest; a wrong one
wastes a bot run.

## Distinguishers — the three pairs that ALWAYS need a tie-break

These overlaps come up often; commit each distinguisher to memory
before you walk the table on a new roadmap item.

### `feature-dev` vs `whole-improve-loop`

- `feature-dev` ships a NEW capability. There is a "done" state
  visible from the outside (a new endpoint, a new UI affordance,
  a new CLI flag). Body reads as a feature spec.
- `whole-improve-loop` improves EXISTING code along an axis
  (reliability, perf, observability, DX). There is no new
  capability — just better/cleaner code. Body reads as a quality
  bar to reach.
- Tie-break: "could a user notice the difference without reading
  the diff?" Yes → `feature-dev`. No → `whole-improve-loop`.

### `sec-audit-*` (DETECTION) vs `whole-improve-loop` (FIX-loop on a security axis) vs `secured-renovacy` (MUTATION on deps)

- `sec-audit-source` / `sec-audit-deps` ARE READ-ONLY. They emit
  findings as kanban issues; they don't fix anything. Use when
  the operator wants a security baseline / list of issues / a
  triage pass — NOT when they want fixes applied.
- `whole-improve-loop` with `improvement_prompt: "security focus"`
  is FIX-mode: alternating review/fix loop until cross-family
  approval. Edits land in the working tree. Use when the operator
  wants security holes closed in place.
- `secured-renovacy` is MUTATION on dependency manifests
  (package.json / go.mod / Cargo.toml / requirements.txt /
  lockfiles). Use when the operator wants CVE patches landed by
  bumping versions, NOT when they want code rewritten to be
  safer.
- Tie-break ladder: "do they want a list?" → sec-audit-*. "do
  they want code rewritten?" → whole-improve-loop. "do they want
  versions bumped?" → secured-renovacy.

### `whole-improve-loop` vs `branch-improve-loop`

- `whole-improve-loop` scans the entire workspace.
- `branch-improve-loop` scans `git diff base_ref...HEAD` only —
  scoped to what the current PR/branch touched, then commits a
  semantic message covering its fixes.
- Tie-break: "is there an open PR / unmerged branch they want
  reviewed?" → `branch-improve-loop`. "is the work
  workspace-wide / no specific branch?" → `whole-improve-loop`.

## When no row matches confidently — three escape hatches

1. **Propose the closest match in rationale, leave `assignee=""`**
   on the item. The body should explicitly say "closest match:
   `<bot>` — operator should confirm before dispatch." This is
   the most common case for cross-cutting or partially-fitting
   work; the operator decides at human_review.
2. **Surface the ambiguity in `rationale`** as a question the
   operator can answer. Example: "Item #3 ('Refactor auth') sits
   between `feature-dev` (new SAML provider as capability) and
   `whole-improve-loop` (reliability/observability on existing
   auth). Pick by replying with the assignee you want, or accept
   the default `""`." The studio renders the rationale verbatim
   so the operator sees the question.
3. **Propose creating a NEW bot** when the catalogue genuinely
   doesn't have a fit and the work will recur. Emit a
   `feature-dev` item whose `feature_prompt` describes the bot
   you'd build (target `.bot` filename, expected vars, pipeline
   sketch). Example: "Build a new bot `flake-hunter` at
   `examples/flake-hunter/main.bot` that runs the test suite N
   times and groups failures by stack trace — needs `vars: {
   suite: string, repeats: int=20 }`."

Bot creation always routes through `feature-dev`; there's no
"bot_factory" assignee. The new bot ships in the same PR as the
item that called for it.

## What ambiguity looks like in practice — examples

- "Improve our auth reliability" → likely `whole-improve-loop`
  with `improvement_prompt: "auth + session handling
  reliability"`, BUT if the operator's priorities mention
  "add OAuth" the same item is `feature-dev`. Surface the
  question if both fits look plausible.
- "Make the docs match the new dispatcher API" → `docs-refresh`
  (clear). No ambiguity.
- "Fix the failing CI on the rust port" → `branch-improve-loop`
  IF there's an open branch, `feature-dev` IF the CI fix is
  itself a new capability (e.g. a new test runner). Surface
  the question.
- "Reduce vendor dependency footprint" → ambiguous.
  `secured-renovacy` could prune by bumping; `whole-improve-loop`
  could refactor to drop dependencies; `feature-dev` could build
  an in-house replacement. Surface as a three-way question.

<!-- ITERION:CATALOG:GENERATED:BEGIN -->

## The team — persona ↔ assignee

When you emit an `assignee`, always use the **technical name** (the
dispatcher routes on it), never the persona.

| Persona | `assignee` (technical name) |
|---|---|
| Billy | `branch-improve-loop` |
| Devy | `devbox-setup` |
| Doki | `docs-refresh` |
| Featurly | `feature-dev` |
| Revi (converse) | `revi-converse` |
| Revi | `review-pr` |
| Depsy | `sec-audit-deps` |
| Seki | `sec-audit-source` |
| Renovacy | `secured-renovacy` |
| Nexie | `whats-next` (this bot) |
| Willy | `whole-improve-loop` |

## Bot reference

### `branch-improve-loop` — Billy

Branch-scoped variant of whole-improve-loop. Runs review-fix iterations
on the changes between a feature branch and its base, auto-commits each
fix, and stops on cross-family double-approval.

- **Use when**:
  Use when an existing branch/PR needs a rigorous review + fix +
  commit before merge. Scopes to git diff base_ref...HEAD and commits
  a semantic message; pass base_ref for a non-main integration base.
- **Vars**: `base_ref` (string), `chunk_dir` (string), `chunk_max_loc` (int), `chunk_threshold_loc` (int), `scope_notes` (string), `workspace_dir` (string)
- **Path**: `bots/branch-improve-loop/main.bot`

### `devbox-setup` — Devy

Bootstraps a reproducible dev environment for a repository. Detects the
project's languages, runtimes, build + test tools and e2e stack (e.g.
Playwright), then authors a PINNED `devbox.json` (Nix-packaged toolchain)
at the repo root and validates it with `devbox install`. The generated
`devbox.json` is what other iterion bots — and humans — use to run the
project's build / test / e2e in a reproducible toolchain (ADR-017 Tier-2/
Tier-3): once a repo has one, `build_rung` / `regress_rung` / patch_author
run project commands via `devbox run -- …`.

Scope discipline: writes ONLY `devbox.json` (+ `devbox.lock`); never edits
source. Default mode proposes the change in a worktree behind a human gate
(the project's dev environment is consequential — an operator confirms
before it lands).

- **Use when**:
  Run on a repo that has NO `devbox.json` yet (so iterion bots can run its
  build/test/e2e reproducibly), or when its toolchain drifted from what the
  code now needs (new language, runtime bump, added e2e). Produces a pinned
  `devbox.json` + `devbox.lock`; it does not change source.
- **Vars**: `workspace_dir` (string)
- **Path**: `bots/devbox-setup/main.bot`

### `docs-refresh` — Doki

Documentation refresh bot. Detects mismatches between project
documentation (README, docs/*.md, CLAUDE.md, bundled skills,
Go code comments) and the actual current state of the code, then
fixes the DOCS (never the code) and auto-commits on convergence.
When a repo has NO documentation yet, it bootstraps an initial
doc set (configurable docs_dir, default "docs") authored from the
code, then refreshes it through the same review loop.

Workflow shape mirrors branch-improve-loop: alternating
claude_code (opus-4-8) and claw (openai/gpt-5.5) reviewers,
deterministic streak_check requiring two cross-family approvals,
prepare_commit + commit_changes phase.

Specificity vs branch-improve-loop: a deterministic upstream
scan_docs tool node enumerates the doc footprint once (find +
sha1sum) so agents cannot truncate the audit set. Reviewers and
fixers operate against this immutable footprint. The fixer is
forbidden from touching anything but `.md` files in scope and
Go code comments — code logic edits are an automatic blocker
on the next iteration.

The bot ships 5 skills capturing the anti-Goodhart rules:
docs-refresh (playbook), doc-mismatch-taxonomy (10-value enum),
doc-scope-enumeration (scanner contract), anti-facade-fix-rules
(substantive fix discipline), doc-verification-checklist (judge
STEP-0 preamble).

- **Use when**:
  Use when README / CLAUDE.md / docs/**/*.md / bundled skills are
  stale versus the code, before a release, or whenever a survey flags
  code↔doc drift — or when a repo has NO docs yet and needs an initial
  set authored from the code. Fixes the DOCS only (never code logic)
  and commits.
- **Vars**: `audit_cache_path` (string), `bundle_self_path` (string), `cli_surface_globs` (string), `code_scope_globs` (string), `coverage_target_pct` (int), `diagnostic_surface_globs` (string), `diff_since` (string), `doc_globs` (string), `docs_dir` (string), `excluded_dirs` (string), `go_comment_globs` (string), `issue_id` (string), `max_drift_candidates` (int), `max_recovery_iterations` (int), `max_review_chunk_docs` (int), `max_review_iterations` (int), `scope_notes` (string), `workspace_dir` (string)
- **Path**: `bots/docs-refresh/main.bot`

### `feature-dev` — Featurly

Autonomous end-to-end feature development. Takes a `feature_prompt`
input, plans (Claude Code, read-only), implements (session-inherit),
invokes /simplify, then runs the alternating Claude/GPT review-fix
loop until two consecutive cross-family approvals.

- **Use when**:
  Use when an item can be phrased as one feature with a clear,
  externally-visible "done" state (new endpoint, UI affordance, CLI
  flag). Also the route for "build a new bot" work — point
  feature_prompt at the new .bot file to author.
- **Vars**: `feature_prompt` (string, required), `workspace_dir` (string)
- **Path**: `bots/feature-dev/main.bot`

### `revi-converse` — Revi (converse)

Conversational sibling of Revi (review-pr). Triggered when an
authorized forge user asks a focused QUESTION on an open merge /
pull request — `/revi <question>` (e.g.
`/revi why is the SSRF critical?`). Reads the question + the MR
diff against the branch's merge-base, formulates a CONCISE,
GROUNDED answer (a senior code reviewer's follow-up — not a
fresh full review), and posts the answer as a REPLY in the same
discussion thread via the forge_token. Never edits, fixes, or
commits code. When `/revi` is sent without a question, the
webhook handler routes to review-pr for a fresh re-review
instead.

- **Use when**:
  Use when an operator asks a follow-up question on an open MR
  about Revi's earlier findings or the diff itself — clarification,
  rationale, severity justification, alternative fixes. NOT for
  re-reviewing the MR (that is review-pr / Revi), NOT for editing
  code (that is Billy or Featurly), NOT for triaging issues on the
  board.
- **Triggers**: revi-converse, ask, converse
- **Vars**: `base_ref` (string), `converse_question` (string), `discussion_id` (string), `pr_url` (string), `replier` (string), `thread_context` (string), `trigger_note` (string), `workspace_dir` (string)
- **Path**: `bots/revi-converse/main.bot`

### `review-pr` — Revi

Read-only cross-family code reviewer. Reviews the working-tree diff
of the current branch against its base with two independent reviewers
(Claude + GPT), merges and de-duplicates their findings (cross-family
agreement raises confidence), then publishes one issue per finding to
the iterion native kanban board (labelled severity + type +
source:revi) and writes a markdown report. Given a pull/merge-request
URL (--var pr_url), it ALSO posts the findings onto that PR as a real
forge review — inline comments anchored to file:line with one-click
```suggestion blocks (GitHub / GitLab / Forgejo). Never edits, fixes,
or commits code — that is the improve-loops' job (Billy / Willy).

- **Use when**:
  Use when you want a PR/branch REVIEWED and its issues surfaced — to
  the board for triage and/or posted directly onto the PR (pass
  --var pr_url) as inline comments + ```suggestion fixes — but NOT
  auto-fixed. Read-only: Revi reports; Billy (branch-improve-loop)
  reviews AND fixes AND commits.
- **Triggers**: review-pr, pr-review, review
- **Vars**: `base_ref` (string), `max_findings` (int), `post_to_board` (bool), `pr_review_mode` (string), `pr_url` (string), `report_path` (string), `scope_notes` (string), `severity_threshold` (string), `workspace_dir` (string)
- **Path**: `bots/review-pr/main.bot`

### `sec-audit-deps` — Depsy

Universal supply-chain malware auditor. Enumerates installed
dependencies per ecosystem (npm/yarn/pnpm, pip/poetry/uv,
go.mod/vendor, …), looks each `(ecosystem, name, version,
checksum)` triple up against a host-wide cache at
`~/.iterion/security-cache/packages.jsonl` to skip packages that
were already analysed at an acceptable scanner version, runs
language-specific static heuristics on the rest (install-time
hooks, eval, obfuscation, fetch+exec, base64 blobs, init()
side-effects), passes the structured signals to an LLM reviewer
with strict JSON output schema (no-package-malware style),
combines heuristic + LLM scores by `max()`, buckets into
LOW/MEDIUM/HIGH, emits findings to the iterion kanban board and
appends a fresh line to the package cache.

Cross-run memory: cache is host-wide and shared across repos
because a published package version is universal. The
`scanner_version` field lets the bot opportunistically rescan
packages analysed by older versions.

Per-language extensibility: ships JS/TS (npm), Python (pip/poetry)
and Go (modules + vendor/), plus a language-agnostic pass on
embedded binaries and locale anomalies. Add a language by dropping
a `skills/lang-<id>.md` and an entry in the `heuristic_scan`
router.

- **Use when**:
  Use for a READ-ONLY supply-chain audit of installed dependencies:
  post-install triage, malware / typosquat / install-hook detection,
  CVE baseline. Emits findings to the board; does not fix.
- **Vars**: `cache_dir` (string), `cache_path` (string), `cache_ttl_days` (int), `scan_dir` (string), `scanner_version` (string), `severity_threshold` (string), `workspace_dir` (string)
- **Path**: `bots/sec-audit-deps/main.bot`

### `sec-audit-source` — Seki

Universal source-code security auditor. Detects languages and
frameworks, runs language-specific SAST (semgrep + gosec / bandit /
npm audit) plus language-agnostic scanners (gitleaks for secrets,
trivy fs for filesystem misconfig, semgrep --config=auto), triages
the raw output with an LLM against a finding taxonomy, confronts
candidates against `.iterion/security/fp-known.yaml` to suppress
curated false positives, revalidates surviving candidates with a
two-phase judge (anti-façade), then writes findings to the iterion
native kanban board (one issue per finding, labelled by severity +
type) and exports a markdown summary.

Cross-run memory: false positives confirmed by the operator (or by
the revalidate judge after explicit human reasoning) are written
back to `.iterion/security/fp-known.yaml` in the repo so the next
run does not re-surface them. Entries are reviewable + editable by
humans.

Per-language extensibility: ships JS/TS, Go, Python and a
language-agnostic baseline. Add a language by dropping a
`skills/lang-<id>.md` and an entry in the `lang_scan` router.

- **Use when**:
  Use for a READ-ONLY security audit of the source itself (injection,
  SSRF, IDOR, broken auth, hardcoded secrets, crypto misuse,
  deserialisation, path traversal, misconfig). Emits findings to the
  board; does not fix. Pre-release hardening / PR-scope review.
- **Vars**: `confirm_threshold` (int), `context_path` (string), `context_ttl_days` (int), `deepsec_concurrency` (int), `deepsec_out` (string), `deepsec_process_limit` (int), `deepsec_root` (string), `diff_base` (string), `enable_deepsec` (bool), `enable_project_context` (bool), `file_filter` (string), `findings_cap_per_file` (int), `force_context_refresh` (bool), `fp_append_policy` (string), `fp_path` (string), `hard_stop_categories` (string), `matchers_dir` (string), `max_fix_per_run` (int), `min_generic_scanners` (int), `patch_attempts` (int), `patch_dir` (string), `records_dir` (string), `records_ttl_days` (int), `remediate` (bool), `remediation_mode` (string), `scan_dir` (string), `scanner_version` (string), `scope_notes` (string), `severity_threshold` (string), `shard_concurrency` (int), `shard_size` (int), `workflow_path` (string), `workspace_dir` (string)
- **Path**: `bots/sec-audit-source/main.bot`

### `secured-renovacy` — Renovacy

Multi-stack agentic dependency upgrade pipeline. Updates every kind of
dependency (libs, languages, frameworks, devops, ci_cd) across every
recognised package ecosystem, aligns consuming code on breaking
changes, cross-references CVE feeds, and runs heuristic malware
detection on the new versions + transitively-introduced libs. Phase 2
closes with an alternating Claude/GPT review/fix loop until cross-
family approval.

- **Use when**:
  Use when dependency risk is the priority: CVE alerts, stale
  lockfiles, version bumps. MUTATES dependency manifests/lockfiles and
  aligns consuming code on breaking changes. Ask before running with
  major_policy: attempt.
- **Vars**: `fix_loop_default` (int), `fix_loop_major` (int), `major_policy` (string), `max_packages_per_run` (int), `override_install_cmd` (string), `override_upgrade_cmd` (string), `scope` (string), `update_scope` (string), `user_prompt` (string), `workspace_dir` (string)
- **Path**: `bots/secured-renovacy/main.bot`

### `whats-next` — Nexie

Orchestrator bot. Surveys the target repo agentically (claw +
openai/gpt-5.5), elicits user priorities (free-text human node),
proposes a long-term roadmap + short-term roadmap + next action +
recommended bots, iterates on free-text human feedback until
approval (bounded revise loop), records the validated plan to disk
(claude_code, native Skill access to the bundled skills), then
asks the human once more whether to auto-invoke the recommended
next bot.

Ships claw + openai/gpt-5.5-generated skills as a dogfood test of
claw-code-go's agentic loop against OpenAI.

- **Use when**:
  Use to decide what to work on next: survey the repo, draft or revise
  a roadmap, and route each item to the right bot (or triage it on the
  board). The orchestrator / entry point, not a worker bot.
- **Vars**: `mode` (string), `scope_notes` (string), `workspace_dir` (string)
- **Path**: `bots/whats-next/main.bot`

### `whole-improve-loop` — Willy

Whole-repository alternating Claude/GPT review-fix loop. Iterates
until two consecutive cross-family approvals, with pushback
protection against false positives. Fragments large workspaces by
package into per-pass token-budgeted chunks (max_review_chunk_tokens,
default 30000) so ~150k+ LoC repos review without context exhaustion;
the coverage-gated streak terminates on a full cross-family clean
sweep — and the streak is persisted across re-dispatches
(.whole_improve_loop.state), so repos whose chunk count exceeds one
run's max_review_passes converge over successive runs instead of
failing forever. See docs/adr/011-whole-improve-loop-context-chunking.md.

- **Use when**:
  Use on EXISTING code when the operator wants a rigorous
  production-readiness audit across the whole workspace, or to drive
  iterative improvement on a specific axis (pass improvement_prompt).
  No new capability — just better/cleaner code.
- **Vars**: `improvement_prompt` (string), `max_review_chunk_tokens` (int), `max_review_passes` (int), `scope_notes` (string), `workspace_dir` (string)
- **Path**: `bots/whole-improve-loop/main.bot`

<!-- ITERION:CATALOG:GENERATED:END -->

## Issue-creation mapping (consumed by `emit_action`)

Each `roadmap_item` lands on the native kanban board as one
issue. The data model on the wire is:

| `roadmap_item` field | Native tracker field | CLI flag (today) |
|---|---|---|
| `title`              | `title`              | `--title`        |
| `body`               | `body`               | `--body`         |
| `assignee`           | `assignee`           | `--assignee`     |
| _(bot name, e.g. `feature-dev`)_ | `bot` (string)       | `--bot` (on `create`) |
| `args` (object)      | `bot_args` (`map[string]string`) | `--bot-arg key=value` (on `create`) |

`bot` and `bot_args` are dedicated typed fields on
[`native.Issue`](../../../pkg/dispatcher/native/issue.go) (JSON
keys `bot`, `bot_args`); they are NOT stored under the freeform
`Fields` map. Set them via `iterion issue create --bot <name>
--bot-arg key=value` (repeatable; values are kept verbatim, so
comma-containing glob lists survive intact), the REST API (POST/PATCH
`/api/v1/native/issues` with `{ "bot": "...", "bot_args": { ... } }`),
or direct `store.Create/Update` calls. `bot_args` is usable today: the
dispatcher merges it on top of the rendered `dispatch.vars`
key-by-key, with `bot_args` winning on shared keys (see
[pkg/dispatcher/loop.go](../../../pkg/dispatcher/loop.go) `buildSpec`).

Concrete `bot_args` example — for an issue assigned to
`feature-dev` with `args = {"feature_prompt": "Add CSV export"}`:

```json
{
  "title": "Add CSV export",
  "assignee": "feature-dev",
  "bot": "feature-dev",
  "bot_args": { "feature_prompt": "Add CSV export" },
  "labels": ["horizon:next-action", "source:whats-next"]
}
```

Horizon labels:

```
horizon=next_action  → --label horizon:next-action --label source:whats-next
horizon=short_term   → --label horizon:short-term --label source:whats-next
horizon=long_term    → --label horizon:long-term --label source:whats-next
```

Operators driving routing only through the CLI today should set
`--assignee <bot_name>` and rely on `assignee_workflows:` /
`assignee_dispatch:` in the dispatcher YAML (or the registry
fallback) to map that assignee to a workflow + var template — see
[docs/dispatcher.md §Routing by issue assignee](../../../docs/dispatcher.md).

## Verification ritual (emit_action)

Before creating each issue:

1. If `assignee != ""`, look it up in the persona table above. If
   it is not one of the listed bots, AND it does not correspond to
   a `.bot` file the explorer surfaced — strip to `""` and add
   label `needs-manual-triage`. NEVER invent an assignee.
2. Empty assignee is FINE. The issue lands without an assignee
   and the operator triages.

## What you do NOT do

- You do NOT shell out `iterion run …` directly. The bot used
  to do that; it doesn't anymore.
- You do NOT enumerate bots from the user's free-text alone.
  Walk the decision tree against the explore summary.
- You do NOT recommend an `assignee` whose card is not in the
  catalog above (and whose `.bot` file the explorer did not
  surface).
- You do NOT recommend more than one `next_action`.

## Backend selection

When authoring a `.bot` (e.g. via `feature-dev`), each agent/judge
node picks where its LLM call runs:

- `backend: "claude_code"` — the official Claude Code CLI. Use for
  nodes that need real tool/shell access (implementers, fixers) or
  the native Skill tool / Claude Code MCP servers.
- `backend: "claw"` — in-process, multi-provider. Use for read-only
  nodes (judges, reviewers, planners) and for any non-Anthropic model
  (`openai/*` models MUST use `backend: "claw"`).
- Omit `backend:` to let the runtime auto-detect from host credentials
  (see [docs/backends.md](../../../docs/backends.md)).

### Per-node `provider:` and the fallback chain

`provider:` is a credential-routing hint, resolved per node after
`${VAR}` expansion. A **single value** routes one credential lane; a
**comma-separated, ordered chain** declares fallbacks that the runtime
walks transparently when a provider fails *beyond its retry budget*:

```yaml
agent reviewer:
  backend: "claude_code"
  provider: "zai,anthropic"        # try z.ai; on hard failure, fall through to Anthropic
  model: "claude-opus-4-8"
```

- Known hints: `anthropic`, `zai`, `openai`, `auto` (≡ default
  precedence). Unknown tokens are warned at compile time (**C087**)
  and ignored at run time.
- On a hard provider failure beyond retries, the executor re-issues the
  same call against the next hint and logs **one** fall-through note —
  the operator sees a route change, not a failure. The run only fails
  if every provider in the chain is exhausted.
- This **generalises `RESCUE_PROVIDER`**: `provider: "${RESCUE_PROVIDER:-zai},anthropic"`
  starts on z.ai (or whatever `RESCUE_PROVIDER` overrides to) and falls
  back to Anthropic automatically — no env flip + manual resume needed.
- The chain is honoured by **`claude_code`** today (same-API family:
  `anthropic`↔`zai`↔Anthropic-compatible facades, identical model id).
  `claw` derives its provider from the `model:` prefix and `codex`
  ignores the hint, so a multi-element chain on those backends is a
  no-op — the runtime uses only the first provider and the compiler
  warns (**C088**). For cross-provider failover on `claw`, vary the
  `model:` instead.
- Single-value `provider:` (and unset) behaves exactly as before —
  the chain form is purely additive.
