---
name: backlog-handoff
description: >
  How Evoly hands proposed evolutions to Nexie and the operator — one
  deep artifact per evolution in the shared findings/ memory inbox, plus
  a dispatch-ready backlog kanban ticket (set_bot + self-contained body)
  the human can launch by dragging to ready.
disable-model-invocation: true
---

# Backlog handoff — two channels, both picked up automatically

Every proposed evolution lands on **two channels** so neither Nexie nor
the operator has to go looking:

- **Channel A — `findings/` memory inbox** (the deep artifact: plan,
  technical decisions, rationale).
- **Channel B — a `backlog` kanban ticket** (the actionable, dispatch-
  ready card).

Both are read by Nexie's next survey with zero changes on Nexie's side.

## Channel A — the findings artifact

For each evolution, `memory_write` to the shared findings scope
(`memory: { scope: "findings" }` → `projects/<key>/memory/findings/`) a
file named `<YYYY-MM-DD>-<slug>.md` with this frontmatter:

```
---
title: "<one-line summary>"
description: "<one sentence>"
kind: "evolution"
source_bot: "evolve"
tags: ["axis:<x>", "horizon:<now|next|later>", "severity:<low|med|high>"]
---

# <title>

## Why
<the rationale, tied to the vision axis it advances>

## Plan
<the technical approach — enough for feature-dev / Nexie to act>

## Technical decisions
<the decisions the operator confirmed that shape this>

## Acceptance
<what "done" looks like>
```

This is the durable, deep record. Nexie's `emit_action` auto-hygiene
later archives it when a resolving commit lands — you do not manage its
lifecycle. Set the evolution_item's `finding_file` to this path so the
ticket body can point at it.

## Channel B — the dispatch-ready backlog ticket

Create one kanban issue per evolution (see `iterion-board.md` for the
tools):

1. `create_issue` — title = the evolution title; **state = `backlog`**
   (the default; do NOT promote to `ready` — that's the operator's or
   Nexie's call); body = a **self-contained spec**.
2. `set_bot` — when a catalog bot clearly fits (e.g. `feature-dev` for a
   self-contained feature), set it. This is the canonical dispatcher
   selector. When no bot clearly fits, leave it unset and add a
   `needs-manual-triage` label.
3. `set_labels` — `source:evolve`, `kind:evolution`,
   `horizon:<now|next|later>`, `axis:<x>`.

### The body IS the dispatch prompt

When the operator drags the ticket to `ready`, the dispatcher routes to
the bot named by `set_bot` and renders **{{issue.title}} + {{issue.body}}**
into that bot's prompt via its `dispatch_vars`. So the body must stand on
its own — a feature-dev ticket's body must be a complete feature spec,
not "see the finding". Include a one-line pointer to `finding_file` for
the deep context, but make the body self-sufficient.

The board MCP cannot set the **typed** `bot_args` map. `set_bot` + a
self-contained body are what make a drag-to-`ready` dispatch correctly.
You may additionally put per-ticket `--var` overrides into
`create_issue` `fields.bot_args` (the board's registered text field) as a
best-effort refinement, but never rely on it as the only channel.

## What Evoly does NOT do at handoff

- Never move a ticket to `ready` — the operator launches it by dragging,
  or Nexie promotes it.
- Never set a hard deadline.
- Never invent a `suggested_bot` you're not confident about — an empty
  bot + `needs-manual-triage` is honest; a wrong bot wastes a dispatch.
- Never assign more than ~10 evolutions in one run.
