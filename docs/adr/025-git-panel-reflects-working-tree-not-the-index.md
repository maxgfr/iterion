# ADR-025: Studio file panel reflects the working tree, not the index

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/status.go](../../pkg/git/status.go) (`collapseStatus`, `parseStatusZ`), [pkg/git/diff.go](../../pkg/git/diff.go) (`Diff`), [pkg/git/worktree_file.go](../../pkg/git/worktree_file.go) (`readWorktreeFile`)

## Context

The studio's modified-files / diff panel answers one question for the
operator: *"what would I see if I opened this file right now?"* — not
*"what is staged?"*. That product framing collides with two git defaults:

- `git status --porcelain` reports **two columns** per file: the index
  state (X) and the worktree state (Y). They can disagree (e.g. `MM` — a
  file modified, staged, then modified again).
- `git show :path` returns the **staged** blob from the index, not the
  dirty working copy on disk.

Bots that run unattended rarely stage selectively — they edit the working
tree and commit wholesale. So the index-vs-worktree distinction is, for
this panel, noise that would actively hide the on-disk reality the operator
is looking at.

## Decision

`collapseStatus` in [`pkg/git/status.go`](../../pkg/git/status.go) reduces
the porcelain `(X, Y)` pair to a **single** status biased to the worktree
column: a `?` in either column means untracked (`??`); otherwise, when the
worktree column `Y` is populated it wins, falling back to the index column
`X` only when `Y` is blank. The operator sees one status per file — the one
that matches what's on disk.

The diff's **After** side is read straight from disk via `os.ReadFile`
(through `readWorktreeFile`, [worktree_file.go](../../pkg/git/worktree_file.go)),
**not** via `git show :path`. As `Diff` in
[`pkg/git/diff.go`](../../pkg/git/diff.go) documents, piping through the
index would mirror staged content and hide the user's unstaged edits — the
opposite of the panel's contract. The Before side still comes from
`git show HEAD:path` (the committed baseline).

This decision governs the **git primitives**. The server's combined-mode
lifecycle tagging (`committed` vs `uncommitted`, see
[ADR-007](007-runview-files-combined-default.md) and
`pkg/server/runs_files.go`) layers **above** these primitives and never
changes the single-column, worktree-biased semantics established here.

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Status columns | one, worktree-biased | two (index X + worktree Y) |
| After-side content | `os.ReadFile` (on-disk) | `git show :path` (staged blob) |
| Matches what the operator sees | yes | only when nothing is staged |
| Selective-staging fidelity | lost | preserved |

The honest concession: this design **discards** the staging distinction. A
user who deliberately stages part of a file and leaves the rest dirty sees
only the on-disk result, not the split — the panel cannot represent a
partial-stage state at all.

## Alternatives considered

### 1. Preserve git's full two-column index-vs-worktree model and read the After side from the index

Surface both porcelain columns in the UI and read staged content with
`git show :path`, faithfully reproducing git's own staging model.

**Rejected because**: the panel is a single-status-per-file list, and the
bots that produce most runs rarely stage selectively. The staging
distinction would be noise in the common case, and reading the After side
from the index would actively **hide** the user's unstaged edits — directly
contradicting the panel's "what's on disk right now" contract.

## Consequences

- **The panel always matches the working tree.** Status and diff content
  reflect what's on disk, which is what the operator opening a file expects
  to see.
- **Partial-stage states are not representable.** A deliberately
  partially-staged file collapses to its on-disk view; the split is
  invisible. Accepted because unattended bots almost never stage
  selectively.
- **`collapseStatus` is the one place the two-column model is flattened.**
  Any future need for the index column has a single, documented seam to
  revisit.
- **Re-challenge — interactive staging / commit-builder UI.** If the studio
  adds interactive staging or a commit-builder, both the discarded index
  column and the staged-content read (`git show :path`) would need to be
  restored — at which point the single-column collapse becomes the wrong
  primitive.
