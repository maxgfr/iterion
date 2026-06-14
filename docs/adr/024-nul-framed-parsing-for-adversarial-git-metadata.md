# ADR-024: NUL-framed (-z) parsing of all git output for adversarial metadata

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/log.go](../../pkg/git/log.go) (`Log`, `parseLog`), [pkg/git/status.go](../../pkg/git/status.go) (`parseStatusZ`), [pkg/git/numstat.go](../../pkg/git/numstat.go), [pkg/git/range.go](../../pkg/git/range.go) (`parseDiffNameStatusZ`)

## Context

The studio's Commits tab and files panel parse `git log`, `git status`,
`git diff --numstat`, and `git diff --name-status` output into typed
structs. The fields these commands emit — commit subjects, author names,
author emails, and file paths — are **user-controlled** and adversarial.
A commit subject can legitimately contain tabs, newlines, pipes, or any
other printable byte; a path can contain spaces and newlines.

If the parser splits on any printable delimiter, a single commit whose
subject contains that delimiter — accidental or malicious — desynchronises
the field offsets and corrupts the parse of the **entire** Commits tab or
files list, not just the one bad row. The panel must be robust to one
pathological commit in the history.

## Decision

All git output in `pkg/git` is parsed in **NUL-delimited (`-z`) form**, on
the single guarantee git makes: a NUL byte (`\x00`) cannot appear in a
commit message or a path.

`Log` in [`pkg/git/log.go`](../../pkg/git/log.go) requests a six-field
record with `--pretty=format:%H%x00%h%x00%s%x00%an%x00%ae%x00%aI` under
`-z`. The `-z` flag suppresses git's default newline between records and
substitutes a NUL; because `%aI` (the date) is the last field, that record
separator **doubles as the sixth field separator** for the next record.
`parseLog` therefore splits the whole stream on NUL and walks it in groups
of six, validating `len(parts) % 6 == 0`.

`parseStatusZ` ([status.go](../../pkg/git/status.go)),
`parseDiffNameStatusZ` ([range.go](../../pkg/git/range.go)), and the
numstat parser ([numstat.go](../../pkg/git/numstat.go)) all consume `-z`
output the same way: `git status --porcelain=v1 -z`,
`git diff --name-status -z`, and numstat split on NUL and handle the
rename/copy case (where the old path is a separate NUL-terminated field).

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Field delimiter | NUL (`\x00`) — git-guaranteed absent from data | tab / `\|` / rare-unicode sentinel |
| Blast radius of one bad commit | none — NUL can't appear in the data | whole-tab parse corruption |
| Record framing | `-z` record-sep doubles as 6th field-sep | explicit trailing delimiter |
| Readability | opaque `%x00` format string | human-readable delimiter |

The cost: the format strings and split logic are harder to read than a
tab-delimited equivalent, and the "record separator doubles as the last
field separator" trick is subtle enough to need this comment-and-ADR to
explain. That opacity is the price of a parser that one adversarial commit
cannot break.

## Alternatives considered

### 1. A printable field delimiter (tab, `|`, or a rare-unicode sentinel) via `--pretty=format`

Use a visible delimiter between fields and a newline between records,
parsing line-by-line and splitting each line on the delimiter.

**Rejected because**: any printable delimiter can legitimately occur inside
a user-controlled subject or author name. A rare-unicode sentinel only
lowers the probability — it does not eliminate it. One adversarial or even
accidental commit containing the delimiter desynchronises the field
offsets and corrupts the parse of the entire output, exactly the
whole-panel failure this design must prevent.

## Consequences

- **One pathological commit cannot break the panel.** NUL is the one byte
  git guarantees absent from messages and paths, so field and record
  boundaries are unambiguous regardless of content.
- **The six-field log format is load-bearing and order-sensitive.** `%aI`
  must remain the last field so the `-z` record separator also terminates
  it; reordering the format string silently breaks `parseLog`'s grouping.
- **Parsers validate framing, not just content.** `parseLog` rejects any
  stream whose field count isn't a multiple of six, turning a framing bug
  into a loud error rather than a mis-attributed row.
- **Re-challenge — structured git output.** Moving to a structured git
  output (a future `--pretty` JSON mode, or libgit2 / go-git object access
  — see [ADR-027](027-git-cli-shell-out-not-embedded-pure-go.md)) removes
  the manual NUL framing entirely.
