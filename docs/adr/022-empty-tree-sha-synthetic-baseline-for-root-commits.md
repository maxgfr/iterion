# ADR-022: Hardcoded empty-tree SHA as synthetic baseline for root commits

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/range.go](../../pkg/git/range.go) (`emptyTreeSHA`, `CommitParent`, `ShowCommit`, `DiffOfCommit`)

## Context

`ShowCommit` and `DiffOfCommit` in
[`pkg/git/range.go`](../../pkg/git/range.go) render the files a single
commit changed, reusing the two-ref `StatusBetween` / `DiffBetween` code
path that powers every other diff in the studio. That path needs a "before"
ref and an "after" ref.

The **first commit of a run** is a root commit: it has no parent, so there
is no "before" ref to diff against. This is not a rare edge case — every
worktree-using run that starts from an empty tree produces one. Without a
baseline, `git diff` against a root commit has nothing to compare, and the
uniform two-ref path breaks precisely where the studio most wants to show
"everything this run introduced".

## Decision

The well-known SHA-1 of git's empty tree object,
`4b825dc642cb6eb9a060e54bf8d69288fbee4904`, is hardcoded as the constant
`emptyTreeSHA` and used as a synthetic "before" ref for root commits.

`CommitParent` resolves a commit's first parent via
`git rev-list --parents -n 1 <sha>` and returns `""` (not an error) for a
root commit. `ShowCommit` and `DiffOfCommit` both check for that empty
parent and substitute `emptyTreeSHA`:

```go
base := parent
if base == "" {
    base = emptyTreeSHA
}
```

Diffing against the empty tree makes every file in the root commit appear
as **Added**, which is exactly the desired rendering, and the commit flows
through `StatusBetween` / `DiffBetween` with no branch in the diff logic
and no special case leaking into the studio's `FilesPanel`.

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Diff code paths | one (empty-tree baseline) | three (root-commit fork in ShowCommit, DiffOfCommit, FilesPanel) |
| Root-commit rendering | every file Added, automatically | hand-built "everything is new" list |
| Coupling to git internals | depends on the SHA-1 empty-tree constant | none |

The constant is the well-known SHA-1 empty tree, stable across every
SHA-1 git repository in existence — but it is a magic number, and the cost
is a hidden dependency on git's object-hash algorithm that the next
maintainer cannot infer from the surrounding code without this record.

## Alternatives considered

### 1. Special-case root commits with a separate "everything is new" branch

Detect the no-parent case in each consumer and build the file list directly
(every path in the commit's tree, marked Added) instead of diffing.

**Rejected because**: the root-commit branch would have to be repeated in
three places — `ShowCommit`, `DiffOfCommit`, and the studio's `FilesPanel`
— forking the diff code path each time. The empty-tree baseline keeps
exactly one path through `StatusBetween` / `DiffBetween` and lets the
existing Added-detection do the work.

## Consequences

- **Root commits render with zero special-casing.** A run's first commit
  shows every file as Added through the same code path as any other diff,
  so the studio's `FilesPanel` needs no awareness of the root case.
- **`CommitParent` returns "" for root commits by design.** Callers must
  treat the empty string as "use the empty tree", not as an error — a
  contract documented on the function.
- **A magic constant carries a hidden hash-algorithm assumption.** The
  value is meaningless without knowing it is git's SHA-1 empty tree; this
  ADR is where that knowledge lives.
- **Re-challenge — SHA-256 repositories.** A git repository using the
  SHA-256 object format has a **different** empty-tree hash, so the
  hardcoded SHA-1 constant breaks the moment iterion targets SHA-256 repos.
  Closing that gap means deriving the empty-tree hash per repo (e.g.
  `git hash-object -t tree /dev/null`) instead of hardcoding it.
