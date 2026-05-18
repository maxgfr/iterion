---
name: session-continuity
description: >
  How to use the iterion workspace memory tools (memory_read,
  memory_write, memory_list) to keep a compact, frontmatter-tagged
  knowledge tree that survives claw's heuristic context compaction.
disable-model-invocation: true
---

# Session Continuity — iterion workspace memory

Three tools, scoped to this bot's memory tree:

- **memory_read(path)** — read a file from the scope.
- **memory_write(path, content)** — write/replace a file in the scope.
- **memory_list(path)** — list files in a scope subdirectory.

The scope lives **outside the repo**, under
`~/.iterion/projects/<encoded-workspace>/memory/<scope>/`. Iterion
**auto-generates an index** of every `.md` file in the scope at
node start (one line per file: path + title + tags + optional
description) and prepends it to your system prompt. You do NOT
maintain an `INDEX.md` by hand — the auto-index always reflects
ground truth.

The bot config may additionally **autoload** the full content of
specific files (e.g. `CONTEXT_BRIEF.md`) on top of the index.
Those are the files you should keep current.

## Writing files

When you call `memory_write`, prefix the body with YAML
frontmatter. The auto-index reads these three keys:

```
---
title: "Operator hard constraints"
description: Running list of constraints captured from feedback.
tags: [constraint, operator, hard]
---

# Body H1 (optional; falls back to frontmatter title)
…
```

- **title** — short label shown in the index.
- **description** — one sentence; surfaces in the index.
- **tags** — kebab-case keywords. Use them so other iterations can
  grep mentally through the index. Conventions:
  - `kind:*` — `kind:decision`, `kind:learning`, `kind:brief`, `kind:question`
  - `topic:*` — `topic:auth`, `topic:roadmap`, `topic:scope`
  - `status:*` — `status:open`, `status:resolved`, `status:revisited`

Frontmatter is **optional**. Without it the index falls back to
the first `# heading` as title, no tags. But tags are the cheapest
way to make a growing memory tree navigable.

## CONTEXT_BRIEF.md (the always-loaded brief)

This is the only file that's typically autoloaded in full. Keep
it under 400 words. Sections:

- **Objective** — one sentence, operator-facing.
- **Hard constraints** — bullets the operator made explicit.
- **Decisions** — durable choices, one-line rationale each.
- **Active files / paths** — recently referenced source files.
- **Open questions** — unresolved items blocking progress.
- **Next action** — single verb-led step.

Update it after every major decision or operator revision. Don't
restate raw artifacts (roadmap JSON, schemas) — those live in
iterion's artifact store.

## When to grow the tree

Beyond CONTEXT_BRIEF.md, create dated/topic-tagged files when:

- A decision has rationale worth preserving (e.g.
  `decisions/2026-05-18-dropped-feature-X.md`).
- A pattern recurs across operator feedback worth noting as a
  learning (e.g. `learnings/code-style.md`).
- You want a long-form note that would bloat the brief.

The auto-index will surface all of these on the next iteration —
no manual bookkeeping needed.

## Path discipline

- Paths are scope-relative. Never absolute, never `../...`.
- Use kebab-case filenames. Date-prefix dated entries.
- One topic per file.

## Do not record

- Secrets (API keys, tokens, OAuth credentials, signed URLs).
- Verbatim schema-typed outputs (artifacts cover that).
- Speculative plans the operator hasn't ratified.
- PII beyond what the operator declared.
