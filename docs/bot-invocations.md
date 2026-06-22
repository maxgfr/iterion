# Bot invocations — how a bot is triggered on a repo

A bot's `manifest.yaml` declares a typed **`invocations:`** block: *how* the bot
can be triggered and *which execution mode* each path uses. It is the
machine-read routing contract, orthogonal to the two pre-existing fields:

| Field | Purpose |
|---|---|
| `forge:` | Forge-access requirements (token scopes, secret name). Optional. |
| `triggers:` (`[]string`) | Free-form advisory labels for the catalog / Nexie. |
| **`invocations:`** | **How the bot is triggered + execution mode.** |

The studio Integrations picker offers every bot that declares an
`invocations:` block (or a legacy `forge:` block); orchestrators that declare
neither (Nexie, Evoly) stay out of the picker.

## Schema

```yaml
invocations:
  - kind: forge | command | schedule | board   # required, closed set
    mode: direct | board                        # optional, default direct
    args_var: <workflow var>                    # optional: where the trigger payload lands
    context_vars: { k: v }                       # optional: vars stamped on every run from this path
    # exactly one payload block, selected by kind:
    forge:    { event: pull_request | pull_request_comment, actions: [opened, reopened] }
    command:  { name: featurly, aliases: [feature-dev], scope: pr|issue|any,
                min_replier_role: maintainer, disambiguator: when_args_empty|when_args_present }
    schedule: { suggested_cron: "0 2 * * 1", default_vars: { k: v } }
```

Validation runs at manifest parse time (`bundle.validateInvocations`): unknown
kind/mode, unknown forge event, malformed command name (`^[a-z][a-z0-9_-]*$`),
bad scope/disambiguator, and intra-bot duplicate command names all fail fast.
`botregistry.ListWithSchema` additionally warns (soft) when an `args_var` names
a var the bot's workflow does not declare.

## Trigger surfaces

- **`forge`** — a webhook event (PR/MR open). Reactive, typically `direct`.
- **`command`** — a `/slash-command` in a PR/MR/issue comment. The universal
  manual trigger, on all three forges (GitLab notes, GitHub/Forgejo
  `issue_comment`). Resolved through the per-webhook command map.
- **`schedule`** — a suggested cron the Integrations UI proposes. *(Cloud
  scheduler not yet wired — the suggestion shows but does not fire; see P3.)*
- **`board`** — a dispatcher target: an issue whose `Bot` is this bot is picked
  up and run.

## Execution mode

- **`direct`** — launch the run immediately (the Revi path: webhook → publisher
  → queue → runner). For fast / read-only / PR-bound work.
- **`board`** — *intended* to materialise a kanban issue assigned to the bot
  and let the dispatcher run it (tracked, retryable, supports human gates). In
  the current build a `board` command still launches **directly** — full
  board-issue materialisation + the cloud board land in a later phase.

## Slash-command routing

`/<name> <args>` in a comment resolves to a bot via the webhook's command map
(computed by the forge orchestrator from the co-enabled bots' command
invocations), with a live `botregistry` fallback for hand-created wildcard
webhooks. Two bots may share a command only when they disambiguate by args
presence — the `review-pr` (bare `/revi`) vs `revi-converse` (`/revi
<question>`) pattern; any other cross-bot collision is rejected at provision
time.

The command args land in the route's `args_var` (e.g. `/featurly add an export
endpoint` → `feature_prompt = "add an export endpoint"`). The commenter is
gated: the bot's own comments are ignored (loop-guard), and the commenter must
be in `AuthorizedRepliers` OR hold a repo role ≥ the command's
`min_replier_role` (GitHub/Forgejo collaborator permission and GitLab access
level are both mapped onto the role scale; default `developer`). Mutating bots
declare `min_replier_role: maintainer`.

## Curated commands

| Bot (persona) | Command | Surfaces | Mode |
|---|---|---|---|
| review-pr (Revi) | `/revi` (bare) | PR open + comment | direct |
| revi-converse | `/revi <question>` | PR comment | direct |
| feature-dev (Featurly) | `/featurly <spec>` | PR/issue | board |
| feature-gap-fill (Fini) | `/fini <gap>` | PR/issue | board |
| branch-improve-loop (Billy) | `/billy` | PR/issue | board |
| whole-improve-loop (Willy) | `/willy <prompt>` | PR/issue | board |
| bmady (Bmady) | `/bmady <brief>` | PR/issue | board |
| secured-renovacy (Renovacy) | `/renovacy` + weekly | PR/issue | board |
| devbox-setup (Devy) | `/devy` | PR/issue | board |
| docs-refresh (Doki) | `/doki` + nightly | PR/issue | board |
| adr-cartograph (Adry) | `/adry` + weekly | PR/issue | board |
| adr-rechallenge (ReArchi) | `/rearchi` | PR/issue | board |
| sec-audit-source (Seki) | `/seki` + Mon 02:00 | PR/issue | board |
| sec-audit-deps (Depsy) | `/depsy` + Mon 03:00 | PR/issue | board |

The technical bot name is always an alias (`/feature-dev` ≡ `/featurly`).

## Adding a new trigger

Adding a way to invoke a bot is a manifest edit — no DSL or engine change.
Drop an `invocations:` entry; the orchestrator picks up the command/events at
the next provision, and the picker shows the new trigger. Keep `args_var`
pointed at a declared workflow var (else the payload is dropped + a warning is
surfaced).

## Key files

- `pkg/bundle/manifest.go` — `Invocation` types + validation; `migrate.go` —
  `SyntheticInvocations`/`EffectiveInvocations`.
- `pkg/webhooks/types.go` — `Config.CommandMap` + `ResolveCommand`;
  `router.go` — `ResolveCommandRoute`.
- `pkg/forge/orchestrator.go` — `buildCommandMap` + events derived from
  invocations (forge: optional).
- `pkg/server/webhooks_gitlab.go` (`handleGitLabCommandNote`),
  `webhooks_prforge.go` (`handlePRForgeComment`), `invocation_dispatch.go`
  (`dispatchInvocation`).
