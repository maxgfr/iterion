---
name: doc-mismatch-taxonomy
description: The 10 enum-locked kinds of doc/code mismatch the docs-refresh bot recognises. Required tag on every blocker.
---

# Doc mismatch taxonomy

Every blocker raised by a docs-refresh reviewer MUST be tagged with one
of these 10 `mismatch_kind` enum values. Hallucinating a new kind
fails schema validation and triggers iterion's parse-fallback retry
path — you will be re-invoked until you pick a valid kind.

If a finding does not fit one of these 10 kinds, it is **not a
blocker for this bot**. Either fit it into the taxonomy or drop it.

| `mismatch_kind` | Description | Example |
|---|---|---|
| `stale_command` | A CLI command, flag, or shell invocation in docs differs from what the code accepts now. | README says `iterion run --foo`, but `--foo` was renamed to `--scope` in [pkg/cli/run.go](pkg/cli/run.go). |
| `wrong_signature` | A function / method / struct field signature shown in docs or a comment differs from the actual code. | Docstring claims `func Foo(ctx, name string)`, code is `func Foo(ctx, name, region string)`. |
| `dead_link` | A markdown link points to a path or anchor that no longer exists. Drift candidates with `kind: md_link` are deterministic detections of exactly this (target file missing, or a `.md` `#heading-anchor` absent under GitHub-slug matching) — confirm them as `dead_link` (`anchor_kind: external`) unless the link is plainly illustrative. | `[see resume docs](docs/resume.md)` but the file was renamed; or `[README](README.md#iter-vs-bot)` where README has no such heading. |
| `removed_file_ref` | Docs reference a file or directory by path that no longer exists, outside markdown link syntax. | README mentions "see `cmd/legacy/main.go` for the old entrypoint" but `cmd/legacy/` was deleted. |
| `stale_behavior_description` | Prose claim about behaviour does not match what the code actually does. | doc says "merges into HEAD by default", code now requires `--merge-into=current` and skips otherwise. |
| `outdated_example` | A code block / example in docs fails to parse, compile, or correspond to the current API surface. | An `.bot` snippet in `docs/grammar/` uses a property that the parser no longer accepts. |
| `wrong_default_value` | Docs claim a default value different from the code's actual default. | docs say `timeout=30s default`, code default is `60s`. |
| `obsolete_capability` | Docs list a feature or capability the code no longer supports (or has not yet shipped). | README lists `--web-ui` as a flag, the web UI was extracted to a separate binary. |
| `wrong_directory_layout` | A directory / package tree shown in docs (e.g. ASCII tree, table of `pkg/` contents) is incomplete or misaligned with the filesystem. | CLAUDE.md's `pkg/` breakdown omits `pkg/dispatcher/native/` which exists on disk. |
| `comment_lies_about_function` | A Go (`//` or `/* */`) comment on a function, method, or package misstates what the code does. Only applies when `go_comment_globs` is non-empty. | `// fetchUser loads a user by ID` but the function loads users by email, not ID. |
| `undocumented_capability` | A code-exposed identifier (CLI command, CLI flag, diagnostic code) exists in the code but no doc in scope lists it. Counter-omission audit direction (code→doc), distinct from `obsolete_capability` (doc lists feature, code removed it). Reviewer extracts the surface from `input.cli_commands`/`cli_flags`/`diagnostic_codes` (v0.6.0). | `cmd/iterion/runner.go` exposes `iterion runner` but `docs/cli-reference.md` doesn't list it. |

## Severity guidance

Independent of kind, blockers also carry a `severity` enum:

- `high` — actively misleading: someone reading the doc and acting
  on it would do the wrong thing (e.g. run a removed command and
  get a confusing error, or follow a wrong example).
- `med` — out of date but unlikely to cause action errors (e.g.
  a tree diagram missing a subdirectory).
- `low` — minor staleness (e.g. an outdated count, "as of 2024"
  marker, etc.). Generally not worth fixing alone; bundle with
  higher-severity work on the same file.

## What is NOT a doc mismatch (do not raise)

- **Style preferences** — wording you'd phrase differently but
  that is technically accurate.
- **Missing documentation** — sections you wish existed. The bot's
  job is alignment, not authoring.
- **Code-side issues** — the bug is in the code, not the doc. Set
  `is_code_bug=true` on the blocker and escalate via `ask_user`;
  do NOT raise it as a fixable mismatch.
- **Spec ambiguity** — when neither doc nor code is clearly
  correct. Escalate via `ask_user`.

## Anchor classification (`anchor_kind` — new in v0.2.0)

Every blocker also carries an `anchor_kind` enum that classifies
the precision of the `code_anchor` citation. This makes the G3
round-trip (next-iteration reviewer re-greps the cited anchor)
mechanically auditable.

| `anchor_kind` | When to use | `code_anchor` shape |
|---|---|---|
| `symbol` | The mismatch is about a named function / type / const / var. The anchor identifies the symbol the reviewer can grep for. | `"pkg/foo/bar.go:Handler"` or `"pkg/foo/bar.go:func Handler"` |
| `line_range` | The mismatch is about a block of code without a clean symbol name (e.g. a config snippet, an inline expression). | `"pkg/foo/bar.go:42-58"` |
| `removed` | The doc claims something exists but it no longer does. The anchor names the former location. | `"pkg/legacy/old.go (removed in commit X)"` or `"<no longer exists>"` |
| `external` | The mismatch is about a non-code claim — e.g. a dead link to a doc file, a stale upstream URL, a wrong directory layout. The anchor is the filesystem/external path the doc references. | `"docs/old-thing.md"` or `"examples/preview_url_demo.bot"` |

Hallucinating a kind outside the enum fails schema validation and
triggers iterion's parse_fallback retry path — pick from the
four values above.

## Consistency contract (v0.4.0 — STRICT)

The combination `(anchor_kind, code_anchor)` must be self-consistent.
Mismatches make the G3 round-trip impossible (a `symbol` kind with
a `<no longer exists>` anchor cannot be re-grepped; an `external`
kind with a `:42` line marker hides whether the anchor is a doc
path or a code citation). Self-check each blocker before submitting:

| If `anchor_kind` is… | `code_anchor` MUST… | …and MUST NOT |
|---|---|---|
| `symbol` | Contain `<path>:<identifier>` with a non-empty identifier (a Go/TS/Python/Rust name, `func X`, `type X`, `const X`, etc.) | Be `<no longer exists>` or contain only a line marker |
| `line_range` | End with `:N` or `:N-M` where N, M are positive integers | Be `<no longer exists>` (use `removed` instead) |
| `removed` | Mention removal (`<no longer exists>`, "removed in commit X", "deleted at version Y") | Cite a current-resolving symbol or line — that's `symbol` or `line_range` |
| `external` | Be a filesystem path or URL that does NOT reference code | Contain `:N` or `:N-M` line markers (the doc/path doesn't have line semantics) |

If you find yourself wanting a fifth kind, pick the closest existing
one and put precision into `code_anchor` itself. The enum is
deliberately locked.

**Fixer-side enforcement**: per `anti-facade-fix-rules.md`, a fixer
that receives a blocker with an inconsistent `(anchor_kind,
code_anchor)` pair pushes it back rather than guessing the
reviewer's intent.

## Output shape (reminder)

Every blocker in `verdict_output.blockers[]` must include:

```
doc_path:        repo-relative path (must be in scan_docs.doc_files)
doc_line_start:  int
doc_line_end:    int
doc_excerpt:     ≤300 chars, exact quote
code_anchor:     string per anchor_kind table above
anchor_kind:     symbol | line_range | removed | external
code_state:      ≤300 chars, what the code actually does now
mismatch_kind:   one of the 10 values above
severity:        low | med | high
suggested_fix:   ≤400 chars proposed new doc text
is_code_bug:     true iff the DOC is correct and the CODE is wrong
```
