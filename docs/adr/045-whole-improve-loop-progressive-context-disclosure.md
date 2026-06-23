# 045 — whole-improve-loop: progressive context disclosure for the reviewer

Status: Accepted (2026-06-23) — implemented behind `context_mode`, default `explore`, validated by run (see docs/bot-runs/whole-improve-loop.md).

## Context

`whole-improve-loop` (Willy) reviews a large repo in per-package chunks. Until
v0.4.0 the deterministic `snapshot_chunk` tool rendered the chunk's **full
source inline** into the reviewer prompt (capped at `max_review_chunk_tokens`),
and the reviewer prompt explicitly **forbade exploration** (`NEVER bulk-read,
glob **/*`) to keep context bounded.

Two problems with the inline model:

1. **Context overflow.** A 30000-token chunk + system prompt + accumulated
   feedback overflowed gpt-5.5's (ChatGPT-forfait) effective window at
   review_loop 11 (run 019ef550, `context_length_exceeded`). Mitigated in
   v0.4.0 by A (lower default 16000) + B (model-adaptive `reviewer_context_tokens`)
   and, in parallel, by the `tail()` accumulator cap (356053e8b). But these only
   *bound* the inline prompt.
2. **A single package/file larger than the reviewer window can never be
   inlined**, no matter how the budget is tuned — the inline model has a hard
   ceiling at the model's context window.

Native agentic coding (Claude Code) does not inline everything: it lists what
exists and the agent **pulls what it needs on demand**, managing its own
context across many tool calls (with compaction). iterion's own doctrine
("skill-guided adaptive agent + deterministic gate verifies the right work
happened") points the same way.

## Decision

Add a `context_mode` (`inline` | `explore`, default **`explore`**) to Willy.

- **`snapshot_chunk`** always writes a per-pass **index markdown** to a
  gitignored workspace path (`.whole_improve_loop.chunks/chunk-<cursor>.md`)
  listing the chunk's files (workspace-relative path + token estimate + the
  chunk label), and emits `chunk_index_path` + `chunk_file_list` (array). In
  `explore` mode it does **not** inline the source (`chunk_content` becomes a
  short pointer); in `inline` mode behaviour is unchanged (the validated v0.4.0
  path remains available).
- The **reviewer** is told the index path and reads the listed files itself via
  its `read_file`/`grep` tools (both `claude_code` and `claw` reviewers already
  carry them), processing incrementally — so a chunk far larger than a single
  inline prompt is reviewable. It outputs `files_reviewed: []` using the **exact
  paths from the index**.
- **Anti-Goodhart guard (deterministic, no new graph node).** The disclosure
  model cannot *guarantee* the model read a file (inline could). The guard, folded
  into the existing `streak_check` compute node using only expr builtins
  (`length`/`unique`/`concat`), requires in `explore` mode that `files_reviewed`
  is **non-empty and a subset of `chunk_file_list`** (`|A∪B| == |A|` ⟺ `B ⊆ A`):
  the reviewer must have engaged with real files from the index and may not
  approve a chunk having read nothing or having cited hallucinated paths. A pass
  that fails the guard does **not** increment `clean_streak` and cannot trigger
  `stop`, so the loop re-reviews until it engages (loop_max is the backstop).
  Full per-file coverage is intentionally **not** hard-required — the operator
  asked to let the reviewer triage "what interests it"; cumulative coverage comes
  from the streak gate + cross-family alternation across passes. A stricter
  full-coverage knob is left as future work.

Everything stays in the `.bot` (pure DSL + Python tool); no engine change.

## Consequences

- Removes the context-window ceiling on chunk size; scales to packages/files
  bigger than any single prompt.
- More tool calls per review (read-on-demand) → higher latency and some
  tool-result tokens, but the reviewer reads selectively and manages its own
  context (compaction), which is the whole point.
- The coverage guarantee weakens from "every line was in the prompt" to "the
  reviewer engaged with real files and the streak/cross-family sweep provides
  cumulative coverage". This is an explicit trade documented here; the
  deterministic engagement guard is the floor.
- `inline` mode is retained (`--var context_mode=inline`) as the conservative
  fallback and for small repos where inlining is cheapest.
- Convergence machinery (streak, cursor rotation, cross-family alternation) is
  unchanged except for the added engagement term in the clean/stop conditions.
