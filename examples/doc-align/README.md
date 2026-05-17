# doc-align

A dogfood-friendly iterion bot that detects mismatches between
project documentation and actual code state, then fixes the
**documentation** (never the code) and auto-commits on convergence.

## What it audits

By default the doc footprint covers:

- `README.md` at repo root
- `docs/**/*.md`
- `CLAUDE.md` (any level)
- `examples/*/skills/*.md`
- Go function- and package-level docstrings (when
  `--var go_comment_globs="..."` is set; empty by default for
  faster first runs)

## Inviolable rules

1. **Docs follow code, never the reverse.** If a doc lies about
   what the code does, the doc gets corrected. If a doc reveals
   what looks like a code bug, the bot escalates to the human
   (`ask_user`) — it never silently rewrites correct docs to
   match buggy code.
2. **The fixer must not touch code.** Allowed extensions:
   `.md` only, plus Go code comments inside files matching
   `go_comment_globs`. Any other file appearing in
   `fix_output.code_files_touched` triggers a high-confidence
   blocker on the next iteration.
3. **The doc footprint is determined by a tool, not an agent.**
   `scan_docs` runs once at the start and emits an immutable
   `doc_files[]` that reviewers/fixers cannot reduce. Agents
   that skip a file must raise a coverage blocker, never
   silently elide it.

## Running

```bash
# From the workspace (worktree recommended for dogfooding).
iterion run examples/doc-align/ \
  --var workspace_dir=$(pwd) \
  --var doc_globs="README.md,CLAUDE.md,docs/**/*.md" \
  --var go_comment_globs="" \
  --var auto_commit=true \
  --var max_review_iterations=10
```

Pass `--var scope_notes="..."` to give the reviewers extra context
about what they should pay extra attention to (e.g. a recent
sweeping refactor).

## Required credentials

- `claude_code` backend reads its own OAuth token (forfait Pro/Max
  or API key — see iterion docs).
- `claw` backend with `openai/gpt-5.5` requires `OPENAI_API_KEY` in
  the environment (or `.env`).

## Convergence

The bot terminates when two consecutive iterations of opposite
families (claude / gpt) both emit `approved=true`, mirroring
`branch_improve_loop`. On convergence, the `prepare_commit` agent
selects the modified files and writes a semantic commit; the
deterministic `commit_changes` tool stages and commits.

Loop bounds: 15 review iterations, 20 recovery (fix) iterations.
