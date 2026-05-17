---
name: doc-scope-enumeration
description: Contract for the scan_docs tool output — agents must treat doc_files[] as the immutable, complete audit footprint.
---

# Doc scope enumeration — the immutable footprint

The `scan_docs` tool node runs ONCE at the very start of a
`doc-align` run, before any reviewer or fixer. It executes a
deterministic shell pipeline (`find` over the configured globs +
`sha1sum` for a footprint hash) and emits:

```
scan_output:
  doc_files:       string[]   # workspace-relative paths, sorted
  doc_count:       int        # len(doc_files)
  footprint_hash:  string     # sha1 of newline-joined sorted paths
  scope_globs:     string[]   # echo of the resolved scope (for transparency)
```

This output is passed to every reviewer as `input.doc_files[]` and
echoed onward through `cumulative_audited_pairs`. It is the
**immutable audit footprint** for the entire run.

## What you must do with it

### As a reviewer

1. Read `input.doc_files[]` literally. Do NOT add files (you would
   exceed your authorisation) and do NOT silently drop files (you
   would defeat the negative-space check).
2. Across iterations, the union of `cumulative_audited_pairs ∪
   audited_pairs` must cover **every** path in `doc_files`. If
   coverage is incomplete, you must NOT vote `approved=true` —
   you must list the uncovered file paths as a blocker with
   `mismatch_kind: ...` only if you actually inspected it and
   found a problem; or you must spend this iteration auditing
   them before voting.
3. If a file in `doc_files[]` was added to the list erroneously by
   the scanner (e.g. a vendored markdown that should not be in
   scope), the correct response is to call `ask_user` to flag the
   scope misconfiguration — not to silently skip the file.

### As a fixer

1. You may only write to paths inside `doc_files[]` and (if
   `go_comment_globs` is non-empty) Go files inside that scope,
   restricted to comment edits.
2. After your fixes, the `fix_output.modified_doc_files[]` you
   report MUST be a subset of `doc_files[]`. Any path outside
   that set triggers a blocker on the next iteration.
3. `fix_output.code_files_touched[]` MUST be empty. The next
   reviewer will check this mechanically.

## Why the scanner is a tool, not an agent

`docs/workflow_authoring_pitfalls.md` documents the failure mode
that led to this design: when the audit set is chosen by an agent,
the agent can rationalise away files it does not want to audit. By
moving file enumeration to a deterministic `find` invocation outside
agent reach, we close the "silent skip" attack vector entirely.

The `footprint_hash` is your evidence that this guarantee was
honoured: if you log it in your verdict reasoning, you've proven
you read the actual scanner output rather than a paraphrase of it.
