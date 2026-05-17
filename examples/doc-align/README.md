# doc-align (v0.7.0)

A dogfood-friendly iterion bot that detects mismatches between
project documentation and actual code state, then fixes the
**documentation** (never the code) and auto-commits on convergence.

**v0.7.0 changes** (GPT session inherit, finally):
- `reviewer_gpt` now declares `session: inherit_if_available` —
  a new iterion runtime mode that behaves as `inherit` when
  `_session_id` resolves to a non-empty value, and silently
  falls back to `fresh` otherwise. Logs which path fired.
- The `alt -> reviewer_gpt` edge wires
  `_session_id: "{{outputs.fix_gpt._session_id}}"`. On iter 1
  (cold) fix_gpt hasn't run and the substitution is empty → run
  fresh. On iter 2+ the reviewer rides fix_gpt's prompt cache,
  cutting per-iter cost roughly 30-50% (v0.2.0 cold reviewer_gpt
  cost up to $7.49; expected $2-4 with cache).
- This is the realisation of the v0.2.0-attempted-and-reverted
  Fix 3. The original revert was correct under the diagnosis-of-
  the-moment (we thought empty session_id broke claw) but the
  real cause was an OpenAI quota issue. With v0.5.0's SSE-error
  surfacing in claw + the new tolerant mode, the optimisation
  is safe to land.

**v0.6.0 changes** (counter-omission audit):
- New deterministic tool node `scan_code_surface` extracts
  "publicly-exposed identifiers" from the workspace via grep:
  CLI commands (cobra `Use:` literals), CLI flags
  (`StringVar`/`BoolVar`/… registrations), and diagnostic codes
  (`Cxxx` constants). Run on this repo it surfaced 26 commands,
  70 flags, 54 diagnostic codes in 50ms.
- New `mismatch_kind` value `undocumented_capability` captures
  the counter-omission case: a code-exposed identifier exists
  but no doc in scope lists it. Distinct from
  `obsolete_capability` (the doc→code direction).
- Reviewers receive the surface lists in `input.cli_commands` /
  `cli_flags` / `diagnostic_codes` and audit code→doc presence
  alongside the existing doc→code audit.
- `--var cli_surface_globs=""` and `--var diagnostic_surface_globs=""`
  both empty disables the surface scan — for library-only repos
  with no CLI or diagnostic surface to document.

**v0.5.0 changes** (inter-run audit cache):
- `scan_docs` now reads `.iterion/doc-align/audit-cache.json`
  (path configurable via `--var audit_cache_path`). For each
  doc whose content sha1 AND every previously-cited code-file
  sha1 are unchanged since the last successful run, the doc is
  emitted in `pre_verified_docs` and seeded directly into the
  coverage gate's `cumulative_audited_docs`. Repeat runs on an
  already-aligned workspace can hit the streak gate on the
  first iteration without re-reading the unchanged majority —
  5-10× speedup on incremental usage (nightly / per-PR).
- New terminal tool node `update_audit_cache` runs after
  `commit_changes` and rewrites the cache from
  `streak_check.cumulative_audited_pairs`. A failed commit
  short-circuits before this node, so the cache only records
  audit state that was actually shipped.
- Cache invalidation is conservative: ANY change to ANY
  referenced code file invalidates the doc's pre-verification.
  No anchor-level precision (we'd need a language-aware
  parser); the trade-off is a slightly higher miss rate for a
  much simpler implementation.
- Empty `audit_cache_path` disables both the read and the
  write — for one-shot CI runs that want a guaranteed fresh
  audit.

**v0.4.0 changes** (anchor_kind tightening):
- `doc-mismatch-taxonomy.md` now carries a STRICT consistency table
  between `anchor_kind` and `code_anchor` shape. A blocker with
  `anchor_kind: symbol` but `code_anchor: "<no longer exists>"`
  is now defined as an inconsistency the fixer pushes back on.
- New `STEP 4b — Anchor consistency self-check` in the reviewer
  checklist; new `Rule 6` in the fixer's anti-façade rules.
- iterion's expr can't validate the json-typed `blockers` field
  mechanically (no JSON parsing in expressions), so the gate is
  prompt-and-skill discipline, ratified through cross-family
  alternation. A future iterion compute primitive could move
  this check into a deterministic node.

**v0.3.0 changes** (lessons from the v0.2.0 dogfood deadlock):
- Streak gate now treats `blocker_count == 0` as effective
  approval. The v0.2.0 streak required `approved=true` from both
  reviewers consecutively; GPT settled into a stable "0 blockers
  + confidence: low" state after substantive findings were
  exhausted, which the v0.2.0 gate didn't accept. The bot
  alternated forever without converging. New verdict field
  `blocker_count: int` (explicit count emitted by the agent —
  needed because iterion's `length()` on a json-typed field
  returns the JSON-encoded char count, not the array size).
- `coverage_pct` is clamped to ≤ 100 in `streak_check`.
  Reviewers sometimes put paths into `audited_docs` outside the
  original footprint (e.g. a `.md` outside `doc_globs` that
  legitimately covers a code surface they were verifying); the
  raw formula pushed the reported coverage to 101+%. Harmless
  for the gate (`>= 80%` still fires) but confusing in reports.
- Fix routing now guards on `blocker_count > 0` instead of
  `length(blockers) > 0` — the latter was effectively a no-op
  because the JSON string is always non-empty even for `[]`.

**v0.2.0 changes** (lessons from the v0.1.0 dogfood):
- G4 gate (fixer never touches code) is now actually wired —
  v0.1.0 routed it via `{{outputs.fix_*.code_files_touched}}`
  in the user prompt, which iterion does not substitute.
- Mechanical coverage gate in `streak_check`: convergence
  requires `coverage_target_pct` (default 80%) of `doc_files`
  to be in `cumulative_audited_docs`. Stops the "Claude
  rubber-stamps after partial audit" failure mode.
- `reviewer_gpt` uses `session: inherit` from the prior
  `fix_gpt` so GPT reviews ride on the prompt cache — major
  cost reduction (v0.1.0 spent $7.49 on a single fresh GPT
  review of the workspace).
- `--var bundle_self_path=examples/doc-align` excludes the bot's
  own bundle from the audit footprint when running on its host
  repo.
- `--var diff_since=<ref>` surfaces recently-changed code files
  as a hint to reviewers (incremental mode).
- New `anchor_kind` enum (`symbol | line_range | removed |
  external`) on every blocker — makes the G3 round-trip
  auditable mechanically.

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
  --var max_review_iterations=10 \
  --var coverage_target_pct=80

# Self-host: when running doc-align on the iterion repo itself,
# exclude the bot's own bundle so it doesn't try to "align" its
# own skills/main.bot.
iterion run examples/doc-align/ \
  --var workspace_dir=$(pwd) \
  --var bundle_self_path=examples/doc-align \
  ...

# Incremental (e.g. nightly): focus on docs that reference
# code changed since a ref.
iterion run examples/doc-align/ \
  --var workspace_dir=$(pwd) \
  --var diff_since=main~7 \
  ...
```

Pass `--var scope_notes="..."` to give the reviewers extra context
about what they should pay attention to (e.g. a recent sweeping
refactor).

## Post-run report

The committed change captures what was aligned. For the full
audit trail — every blocker raised, every audited_pair, every
fix iteration's `summary` — use the built-in report command:

```bash
iterion report --run-id <run_id> --output report.md
```

`<run_id>` is printed at the top of every `iterion run`
invocation (`Run ID: run_<timestamp>`). The generated `report.md`
includes per-node costs, the final coverage_pct, and the
chronological event stream.

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
