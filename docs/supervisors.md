# Supervisor agents

A **supervisor** is an LLM agent that watches another running agent and
enqueues steering messages the supervised agent picks up **at its next
turn** — exactly like a human operator watching a Claude Code session in
the studio and typing a quick correction mid-work. The supervisor runs
*concurrently* and reacts to the supervised run's live activity.

Supervisors are **node-scoped**: one supervisor watches one or more
*agent nodes* (e.g. `implement`, `fix`), not necessarily the whole run.
It is *armed* only while one of its watched nodes is the active node, and
every message it injects is tagged with that node so a late message can't
leak into the next node. Watching the whole run is the degenerate case
(no node filter).

## How it works

```
supervised run ──events──▶ Coordinator ──wake──▶ Supervisor bot (LLM)
                               │                        │
                     monitors + cooldown          decision: enqueue
                               │                        │
                               └──── QueueMessage ◀──────┘
                                        │
                          drained at the supervised node's NEXT turn
```

The coordinator (`pkg/supervise`) subscribes to the run's event stream
(`runview.Service.ObserveRun`), tracks the active node, and wakes the
supervisor bot on:

- **turn boundaries** (`llm_step_finished` / `node_finished` /
  `node_started`), debounced and rate-limited by a cooldown, and
- **monitor matches** — event patterns the bot registers interest in
  (a Bash failure, an edit to a path, a cost threshold), which fire
  immediately, bypassing the cooldown.

On each wake the bot returns a structured **decision**: whether to
`intervene` now (and with what `message`), which event patterns to keep
`watch`-ing, and whether it is `done`. When it intervenes, the message is
enqueued via the same `runview.Service.QueueMessage` path operator chat
uses — so it is delivered at the next turn boundary (the claude_code
inbox-drain hooks / the claw `InboxDrain` closure), shows in the studio
run conversation, and is **node-scoped** (`store.QueuedUserMessage.NodeID`).

A hard `max_evals` budget and the cooldown keep token cost bounded;
supervision degrades to a silent no-op when the budget is exhausted —
it never eats the supervised run's budget.

## Attaching a supervisor to a running run (CLI)

```sh
iterion supervise --run-id <id> \
  --node implement \
  --system @policies/watchdog.md \
  --monitor event_type=tool_error,tool_name=Bash \
  --model anthropic/claude-opus-4-8
```

Flags:

| flag | meaning |
|------|---------|
| `--run-id` | the run to supervise (required) |
| `--node` | agent node id(s) to watch (repeatable; empty = whole run) |
| `--system` | supervision policy text, or `@path` to read it from a file |
| `--model` | supervisor model spec; default auto-detect or `ITERION_DEFAULT_SUPERVISOR_MODEL` |
| `--monitor` | pre-declared monitor `key=val,key=val` (repeatable). Keys: `event_type`, `node_id`, `tool_name`, `text_contains`, `cost_gt` |
| `--cooldown` | min time between LLM evals on turn boundaries (default 30s) |
| `--max-evals` | hard cap on LLM evaluations for the run (default 20) |

The supervisor blocks until the run terminates or you Ctrl-C to detach.
Because it observes via the shared store, it works against a run launched
by any other process (a `iterion run`, the studio, the dispatcher).

## Monitors

A monitor is an event pattern; every set field must match (unset fields
are wildcards):

- `event_type` — matches the event type verbatim (`tool_error`,
  `node_finished`, `budget_warning`, …)
- `node_id` — matches the event's node
- `tool_name` — matches a tool event's tool (`Bash`, `Edit`, …)
- `text_contains` — case-insensitive substring against the rendered event
- `cost_gt` — fires on a `budget_warning` whose `used` exceeds the value

The supervisor bot registers the few signals it cares about and is then
woken only when they fire, instead of re-reading every turn — this is the
main token-saver.

## Limits

- **Next turn, not now.** A message lands at the next tool/stop boundary
  of the supervised node; an in-flight LLM call is never interrupted.
- **Local store / broker mode.** `ObserveRun` is wired for the local
  broker path; cloud event-source mode is a follow-up.

## Roadmap

- **In-`.bot` `supervisor <name>:` declaration** — declare a supervisor
  inline in the workflow it watches (`watches: [implement]`, `model:`,
  `system:`), auto-spawned by the engine when the run starts. The
  primary authoring path; the CLI above is the attach path for runs that
  did not declare one.
- **Raw Claude Code session bridge** — supervise a raw `claude` CLI/VSCode
  session (no `.bot`) by tailing its transcript and injecting via a
  `settings.json` Stop/PostToolUse hook that drains an iterion inbox.
