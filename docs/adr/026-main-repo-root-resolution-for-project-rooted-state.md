# ADR-026: Main-repo-root resolution (gitdir pointer parse) for project-rooted state

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/range.go](../../pkg/git/range.go) (`FindRepoRoot`, `FindMainRepoRoot`, `parseGitfilePointer`)

## Context

Run and dispatcher worktrees live **under** the repo they belong to:
`<repo>/.iterion/worktrees/<id>` for runs,
`<repo>/.iterion/dispatcher/workspaces/<id>` for dispatched issues. These
are git *linked worktrees* — their `.git` is a file pointing at
`<main>/.git/worktrees/<name>`, not a real gitdir.

Two different consumers need a "repo root" from a starting directory, and
they need **different** roots:

- Recovering a run's working context wants the worktree itself.
- Project-rooted shared state — workspace memory, findings — must key on
  the operator's **main checkout**. If it keys on the worktree, every run
  and every dispatched issue gets its own isolated state tree, invisible to
  a `whats-next` session running at the actual repo root. The shared
  knowledge the feature exists to provide would fragment per-worktree.

A single repo-root notion cannot serve both.

## Decision

[`pkg/git/range.go`](../../pkg/git/range.go) provides **two** resolvers.

`FindRepoRoot` walks parent directories from the start dir until it finds a
valid worktree root (`isGitWorkTreeRoot` — `.git` present and
`git rev-parse --show-toplevel` succeeds), returning the **worktree**
itself. On a linked worktree it returns the worktree path, not the main
repo.

`FindMainRepoRoot` builds on it. After finding the worktree, it stats
`.git`: a **directory** means this is the main checkout, returned as-is. A
**file** means a linked-worktree pointer — it reads the file, parses the
single `gitdir: <path>` line (`parseGitfilePointer`), and since that path
is `<main>/.git/worktrees/<name>`, walks **three `filepath.Dir` jumps up**
(drop the worktree name, then `worktrees/`, then `.git/`) to reach the main
repo. The result is sanity-checked with `isGitWorkTreeRoot` before being
returned.

Runtime, sandbox, and memory keying call `FindMainRepoRoot`. Crucially, on
**any** parse, stat, or read failure on the pointer file — and when the
three-up walk doesn't land on a valid worktree root — it **falls back to
returning the worktree path**, the same legacy behaviour callers had before
the helper existed. The hardening never makes state keying worse than it
was.

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Resolvers | two (`FindRepoRoot` + `FindMainRepoRoot`) | one shared repo-root notion |
| State scope | keyed on the main checkout | per-worktree, fragmented |
| Main-repo discovery | parse `.git` pointer file directly | `git rev-parse --git-common-dir` |
| Failure mode | graceful fallback to worktree | hard error / wrong key |

The cost: two resolvers callers must choose between correctly — pick
`FindRepoRoot` where `FindMainRepoRoot` was needed and shared state
silently fragments again. The choice is a permanent correctness burden the
single-resolver design wouldn't carry.

## Alternatives considered

### 1. A single repo-root notion (the worktree) used everywhere

Resolve one repo root — the nearest worktree — and key all state on it.

**Rejected because**: it produces per-worktree findings and memory trees
that are invisible to a session running at the operator's actual repo root.
The shared, project-rooted state that workspace memory exists to provide
would fragment across every run and dispatched-issue worktree.

### 2. `git rev-parse --git-common-dir`

Ask git itself for the common gitdir shared between a worktree and its main
repo, then derive the main checkout from it.

**Rejected because**: it was passed over in favour of the pointer-file parse
that **degrades gracefully** to the worktree on any failure. The direct
parse keeps the resolver pure-filesystem (no extra git spawn) and gives a
defined fallback, whereas a failed `rev-parse` would need its own error
handling to avoid mis-keying state.

## Consequences

- **Shared state keys on the operator's main checkout.** Workspace memory
  and findings written from any run or dispatched-issue worktree are
  visible to a `whats-next` session at the real repo root.
- **Callers must choose the right resolver.** `FindMainRepoRoot` for
  project-rooted state, `FindRepoRoot` for worktree-local context; the
  wrong choice silently fragments shared state.
- **Failures degrade to legacy behaviour, never worse.** Any pointer-file
  parse failure falls back to the worktree path, so the hardening cannot
  regress a previously-working setup.
- **Re-challenge — run state outside the repo tree.** If run state moves
  entirely out of the repo tree (e.g. always under `~/.iterion` keyed by a
  repo hash), the worktree-vs-main distinction for state keying collapses
  and one resolver suffices again.
