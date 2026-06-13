# feature_gap_fill

Gap-driven feature completer. Specialisation of `feature_dev` (Featurly):
the input is a STRUCTURED gap spec ("here is what's implemented, here is
what's missing") rather than a feature description from zero. Fini reads
the partial implementation, completes the missing parts, runs the
alternating Claude/GPT review-fix loop until two consecutive cross-family
approvals, then commits.

## When to use

- An ADR-driven survey (Adry / `adr-cartograph`) emitted a
  `type:feature-gap` issue with a structured gap spec.
- An operator wants to FINISH a known partial implementation without
  re-architecting what already works.
- Prefer `feature_dev` (Featurly) for greenfield work where there is no
  existing partial implementation to preserve.

## Inputs

| Var | Required | Description |
|---|---|---|
| `gap_spec` | yes | Structured gap spec describing what's implemented vs what's missing |
| `workspace_dir` | no | Defaults to `${PROJECT_DIR}` (the run's worktree) |

A gap spec typically lists:
- `implemented[]` — files / abstractions already in place (preserve)
- `missing[]` — the concrete deliverables Fini must add
- `evidence[]` — references (paths, line numbers) that anchor the survey

## Pipeline

1. `survey_existing` — Claude Code, read-only survey of the partial
   implementation referenced by the gap spec → `existing_state`.
2. `plan` — Claude Code, read-only, gap-aware planning that layers the
   missing parts on top of `existing_state.abstractions_in_place`.
3. `act` — Claude Code, session-inherit, implements ONLY the missing
   parts. Preservation discipline: do not touch files in
   `existing_state.what_works` unless required to wire up the missing
   parts.
4. `simplify` — Claude Code, native `/simplify` skill on the new code.
5. `alt → reviewer_claude / reviewer_gpt → streak_check → fixers` —
   verbatim alternating review/fix loop, stops on cross-family double
   approval.
6. `prepare_commit` → `commit_changes` — semantic commit, trailer
   `Bot: feature-gap-fill`.

## Run

```bash
iterion run bots/feature-gap-fill/main.bot \
  --var gap_spec='implemented: [pkg/foo/api.go, pkg/foo/types.go]; missing: [pkg/foo/handler.go for POST /foo, tests in pkg/foo/handler_test.go]; evidence: [pkg/foo/api.go:42 declares the route surface]'
```

See [main.bot](main.bot) for the full DSL.
