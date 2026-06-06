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
     reasoning you maintain by hand. The generated copy Nexie actually
     reads is the sibling iterion-bot-catalog.md. -->

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
| "implement feature X", "add capability", "build the thing" | `feature_dev` |
| "build a new bot for Y" / "create a workflow that does Y" — the catalogue lacks a fit and we need to author one | `feature_dev` (with `feature_prompt` pointing at the new `.bot` file to create) |
| "review the whole codebase", "audit production-readiness", "find bugs anywhere" | `whole_improve_loop` |
| "focus on axis X" (observability / perf / DX / refactoring) ACROSS the codebase — improvement loop, not detection | `whole_improve_loop` (with `--var improvement_prompt=…`) |
| "review this branch", "review the PR", "fix the diff against main" — review AND fix AND commit | `branch_improve_loop` |
| "review this PR / branch and just REPORT the issues" — read-only review, posts findings to the board, does NOT fix or commit | `code_review` |
| "upgrade dependencies", "patch CVEs", "bump versions", "renovate" — MUTATING (writes package.json / go.mod / lockfiles) | `secured-renovacy` |
| "audit the docs", "find code↔doc drift", "doc/code alignment", "fix outdated README/CLAUDE.md" | `doc-align` |
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

### `feature_dev` vs `whole_improve_loop`

- `feature_dev` ships a NEW capability. There is a "done" state
  visible from the outside (a new endpoint, a new UI affordance,
  a new CLI flag). Body reads as a feature spec.
- `whole_improve_loop` improves EXISTING code along an axis
  (reliability, perf, observability, DX). There is no new
  capability — just better/cleaner code. Body reads as a quality
  bar to reach.
- Tie-break: "could a user notice the difference without reading
  the diff?" Yes → `feature_dev`. No → `whole_improve_loop`.

### `sec-audit-*` (DETECTION) vs `whole_improve_loop` (FIX-loop on a security axis) vs `secured-renovacy` (MUTATION on deps)

- `sec-audit-source` / `sec-audit-deps` ARE READ-ONLY. They emit
  findings as kanban issues; they don't fix anything. Use when
  the operator wants a security baseline / list of issues / a
  triage pass — NOT when they want fixes applied.
- `whole_improve_loop` with `improvement_prompt: "security focus"`
  is FIX-mode: alternating review/fix loop until cross-family
  approval. Edits land in the working tree. Use when the operator
  wants security holes closed in place.
- `secured-renovacy` is MUTATION on dependency manifests
  (package.json / go.mod / Cargo.toml / requirements.txt /
  lockfiles). Use when the operator wants CVE patches landed by
  bumping versions, NOT when they want code rewritten to be
  safer.
- Tie-break ladder: "do they want a list?" → sec-audit-*. "do
  they want code rewritten?" → whole_improve_loop. "do they want
  versions bumped?" → secured-renovacy.

### `whole_improve_loop` vs `branch_improve_loop`

- `whole_improve_loop` scans the entire workspace.
- `branch_improve_loop` scans `git diff base_ref...HEAD` only —
  scoped to what the current PR/branch touched, then commits a
  semantic message covering its fixes.
- Tie-break: "is there an open PR / unmerged branch they want
  reviewed?" → `branch_improve_loop`. "is the work
  workspace-wide / no specific branch?" → `whole_improve_loop`.

## When no row matches confidently — three escape hatches

1. **Propose the closest match in rationale, leave `assignee=""`**
   on the item. The body should explicitly say "closest match:
   `<bot>` — operator should confirm before dispatch." This is
   the most common case for cross-cutting or partially-fitting
   work; the operator decides at human_review.
2. **Surface the ambiguity in `rationale`** as a question the
   operator can answer. Example: "Item #3 ('Refactor auth') sits
   between `feature_dev` (new SAML provider as capability) and
   `whole_improve_loop` (reliability/observability on existing
   auth). Pick by replying with the assignee you want, or accept
   the default `""`." The studio renders the rationale verbatim
   so the operator sees the question.
3. **Propose creating a NEW bot** when the catalogue genuinely
   doesn't have a fit and the work will recur. Emit a
   `feature_dev` item whose `feature_prompt` describes the bot
   you'd build (target `.bot` filename, expected vars, pipeline
   sketch). Example: "Build a new bot `flake-hunter` at
   `examples/flake-hunter/main.bot` that runs the test suite N
   times and groups failures by stack trace — needs `vars: {
   suite: string, repeats: int=20 }`."

Bot creation always routes through `feature_dev`; there's no
"bot_factory" assignee. The new bot ships in the same PR as the
item that called for it.

## What ambiguity looks like in practice — examples

- "Improve our auth reliability" → likely `whole_improve_loop`
  with `improvement_prompt: "auth + session handling
  reliability"`, BUT if the operator's priorities mention
  "add OAuth" the same item is `feature_dev`. Surface the
  question if both fits look plausible.
- "Make the docs match the new dispatcher API" → `doc-align`
  (clear). No ambiguity.
- "Fix the failing CI on the rust port" → `branch_improve_loop`
  IF there's an open branch, `feature_dev` IF the CI fix is
  itself a new capability (e.g. a new test runner). Surface
  the question.
- "Reduce vendor dependency footprint" → ambiguous.
  `secured-renovacy` could prune by bumping; `whole_improve_loop`
  could refactor to drop dependencies; `feature_dev` could build
  an in-house replacement. Surface as a three-way question.

<!-- ITERION:CATALOG:GENERATED:BEGIN -->
<!-- ITERION:CATALOG:GENERATED:END -->

## Issue-creation mapping (consumed by `emit_action`)

Each `roadmap_item` lands on the native kanban board as one
issue. The data model on the wire is:

| `roadmap_item` field | Native tracker field | CLI flag (today) |
|---|---|---|
| `title`              | `title`              | `--title`        |
| `body`               | `body`               | `--body`         |
| `assignee`           | `assignee`           | `--assignee`     |
| _(bot name, e.g. `feature_dev`)_ | `bot` (string)       | `--bot` (on `create`) |
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
`feature_dev` with `args = {"feature_prompt": "Add CSV export"}`:

```json
{
  "title": "Add CSV export",
  "assignee": "feature_dev",
  "bot": "feature_dev",
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

When authoring a `.bot` (e.g. via `feature_dev`), each agent/judge
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
