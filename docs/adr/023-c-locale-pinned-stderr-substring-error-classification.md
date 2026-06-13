# ADR-023: C-locale-pinned stderr substring matching for git error classification

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/git.go](../../pkg/git/git.go) (`gitEnv`, `run`, `ErrNotGitRepo`), [pkg/git/range.go](../../pkg/git/range.go) (`showAt`), [pkg/git/diff.go](../../pkg/git/diff.go) (`errNotInHead`)

## Context

The wrapper must distinguish several git outcomes that are not errors from
the studio's point of view, and render each differently:

- **"not a git repository"** — the target directory has no checkout; the
  HTTP layer should answer `200 { available: false }`, not `500`.
- **"path missing at this ref"** — the file is untracked, freshly added, or
  deleted; the diff should show a nil side, not fail.
- **"unknown / bad revision"** — the ref doesn't resolve; again a nil side.

git does not surface these as distinct, stable **exit codes** through the
single `run` seam (see [ADR-021](021-is-ancestor-via-merge-base-equality.md)
for the same constraint applied to ancestry) — they all arrive as a generic
non-zero exit. The only signal that tells them apart is git's **stderr
text**. That text is localized: under a non-English locale (e.g. `fr_FR`)
git prints `« … n'est pas un dépôt git »`, and an English substring match
silently fails, mid-classifying a clean "no repo" as a hard error.

## Decision

`gitEnv` in [`pkg/git/git.go`](../../pkg/git/git.go) pins `LC_ALL=C` and
`LANG=C` on **every** git invocation — `run`, `ShallowClone`, and the
`showAt` direct `exec.Command` — so git's user-facing messages stay
English regardless of the operator's locale.

On top of that stable text, classification is substring matching:

- `run` maps stderr containing `not a git repository` to the sentinel
  `ErrNotGitRepo`.
- `showAt` in [`pkg/git/range.go`](../../pkg/git/range.go) maps
  `does not exist`, `exists on disk, but not in`, `unknown revision`, and
  `bad revision` to `errNotInHead` (defined in
  [`pkg/git/diff.go`](../../pkg/git/diff.go)), which callers render as a nil
  diff side.

The locale pin is what makes the substring matching sound; without it the
matching is a latent locale-dependent bug.

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Outcome signal | stderr substring (English-pinned) | exit code / structured porcelain |
| Locale safety | `LC_ALL=C`/`LANG=C` on every call | inherits operator locale |
| Coupling | depends on git's English message wording | depends only on documented exit codes |

The cost is real and worth stating plainly: the classifier is coupled to
the **exact wording** of git's English error messages. A future git release
that rephrases "does not exist" breaks a branch silently, and the pinned
locale must be remembered on every new git call site or the bug returns.

## Alternatives considered

### 1. Classify outcomes by exit code or structured porcelain

Branch on git's exit status, or use a machine-readable output mode, so the
classification is locale-independent and wording-independent.

**Rejected because**: git does not expose these particular conditions
("not a git repo", "path missing at ref", "bad revision") as distinct,
stable exit codes through the single centralized `run` seam — they all
collapse to a generic non-zero exit, and there is no porcelain mode that
reports them as typed statuses. Substring matching on stderr is the only
available signal, which in turn forces the locale pin to make that signal
deterministic.

## Consequences

- **Error classification is deterministic across locales.** An operator
  running under `fr_FR`, `de_DE`, etc. gets the same `available: false` /
  nil-diff-side rendering as one running under `C`.
- **Every new git call site must pin the locale.** `gitEnv` must be set on
  any future `exec.Command("git", …)` added to the package, or that call's
  stderr classification regresses under non-English locales.
- **The classifier is coupled to git's English message wording.** A git
  release that rephrases a matched substring will silently mis-classify;
  the substring list is a maintenance liability documented here.
- **Re-challenge — machine-readable git errors.** If git gains stable
  machine-readable error codes, or iterion adopts a structured-error git
  interface (libgit2 / go-git — see
  [ADR-027](027-git-cli-shell-out-not-embedded-pure-go.md)), both the
  locale pin and the substring matching disappear.
