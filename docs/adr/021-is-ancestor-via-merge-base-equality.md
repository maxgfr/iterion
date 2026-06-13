# ADR-021: IsAncestor via merge-base equality, not --is-ancestor plumbing

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/range.go](../../pkg/git/range.go) (`IsAncestor`, `MergeBase`), [pkg/git/git.go](../../pkg/git/git.go) (`run`)

## Context

After a worktree-using run finalizes, the studio must decide whether to
offer a squash-merge of the run's `FinalCommit` into the target branch. If
the commit is *already* reachable from the target branch — an out-of-band
merge happened — offering the squash again is redundant and confusing, so
the studio needs an ancestry test.

git ships a purpose-built plumbing command for exactly this:
`git merge-base --is-ancestor A B` exits `0` when A is an ancestor of B and
`1` when it is not. But every git call in this package routes through one
centralized helper, `run` in [`pkg/git/git.go`](../../pkg/git/git.go),
which collapses **any** non-zero exit into a Go `error`. Exit code `1` ("not
an ancestor" — a normal, expected answer) is therefore indistinguishable
from exit code `128` ("bad object", a real failure) without special-casing
`run` to inspect the exit status.

## Decision

`IsAncestor` in [`pkg/git/range.go`](../../pkg/git/range.go) avoids
`--is-ancestor` entirely. It computes `MergeBase(ancestor, descendant)` and
tests whether the merge base equals `ancestor` itself: A is an ancestor of
B iff `merge-base(A, B) == A`. To make the equality hold for branch names
and short SHAs (not just full SHAs), it first normalises `ancestor` to a
full commit SHA via `git rev-parse --verify --quiet <ancestor>^{commit}`.

Every git invocation in the path — `merge-base`, `rev-parse` — flows
through the same `run` helper. The function returns `false` on any git
error and when the two refs share no common history (`MergeBase` returns
`""`), so a non-ancestor and a failed lookup both yield the safe "don't
claim ancestry" answer.

## Trade-offs

The merge-base-equality form costs one extra `git` invocation (the
`rev-parse` normalisation) and is less self-documenting than the plumbing
command it replaces — a reader has to know the `merge-base(A,B) == A`
identity. What it buys is a single error-handling seam: there is no
exit-code-aware branch anywhere in the package, so `run` stays the one
place that turns git failures into Go errors. The honest concession: the
code is doing manually what a one-line plumbing call expresses directly,
purely because the wrapper cannot surface exit code 1 as a non-error.

## Alternatives considered

### 1. `git merge-base --is-ancestor A B`

Call the purpose-built plumbing and branch on its exit code: `0` ⇒ yes,
`1` ⇒ no, anything else ⇒ error.

**Rejected because**: the centralized `run` helper collapses every non-zero
exit to a Go error, so "not an ancestor" (exit 1) cannot be told apart from
a real failure (exit 128) without special-casing `run` to expose the
underlying `*exec.ExitError` — a change that would ripple to every caller
and undermine the single-error-seam invariant.

## Consequences

- **The single-error-seam invariant is preserved.** No caller in `pkg/git`
  inspects a raw exit code; `run` remains the only translation point from
  git failure to Go error.
- **Ancestry failures are safe by default.** Any git error, a missing
  common ancestor, or an unresolvable `ancestor` ref all return `false`, so
  the studio errs toward *not* claiming a redundant merge.
- **One extra process spawn per check.** The `rev-parse` normalisation runs
  on every call; acceptable for a UI-triggered, low-frequency check.
- **Re-challenge — run() exposing exit codes.** If `run` is ever refactored
  to return the underlying `*exec.ExitError` (or a typed exit code), the
  cleaner `--is-ancestor` plumbing can replace this merge-base-equality
  workaround and the `rev-parse` normalisation disappears with it.
