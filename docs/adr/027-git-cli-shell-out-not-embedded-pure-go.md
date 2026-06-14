# ADR-027: pkg/git shells out to the host git binary, not an embedded pure-Go library

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/git.go](../../pkg/git/git.go) (`run`, `gitEnv`), [pkg/git/clone.go](../../pkg/git/clone.go) (`ShallowClone`)

## Context

iterion needs `git status` / `diff` / `log` for the studio's modified-files
panel and a shallow clone for bot-bundle install. There is a real fork in
the road for how a Go program obtains git operations: shell out to the host
`git` binary, or embed a pure-Go implementation
([go-git/go-git](https://github.com/go-git/go-git)).

This is not a hypothetical axis for this codebase — it is a *recorded* one.
[ADR-003](003-privacy-tools-pure-go.md) chose pure-Go privacy tools over a
Python/ONNX sidecar for a different subsystem, on the grounds of dropping an
external runtime dependency. So "embed in Go vs depend on an external
runtime" is a live decision the project has weighed before and could
reasonably be expected to weigh the same way again here. The opposite
choice for `pkg/git` therefore needs its own record.

## Decision

`pkg/git` is a **thin wrapper that shells out to the host `git` binary** for
every operation. `run` in [`pkg/git/git.go`](../../pkg/git/git.go) is the
single seam: it builds `exec.Command("git", …)`, sets the working
directory, pins the locale
([ADR-023](023-c-locale-pinned-stderr-substring-error-classification.md)),
and returns combined stdout. No git object format, packfile, or refs logic
lives in Go.

`ShallowClone` in [`pkg/git/clone.go`](../../pkg/git/clone.go) deliberately
delegates **network and authentication** to git: it runs
`git clone --depth 1 --single-branch -- <url> <dest>` so the host's
configured credential helpers, `~/.gitconfig`, and SSH keys apply exactly
as they would for a manual `git clone`. iterion writes no auth code of its
own for clone.

## Trade-offs

| Dimension | Chosen approach (shell out) | Rejected approach (go-git) |
|---|---|---|
| status/diff parity with user's CLI | exact (same binary, same config) | diverges (gitattributes, autocrlf, .gitignore) |
| Credential helpers / SSH | inherited for free | must be reimplemented |
| Runtime dependency | requires `git` on PATH | none — pure Go |
| Error/data interface | parse text output (see [ADR-024](024-nul-framed-parsing-for-adversarial-git-metadata.md)) | structured Go objects |
| Build / binary size | smaller; no git library vendored | larger; vendored implementation |

The honest concession: shelling out makes the host `git` binary a hard
runtime dependency. In an environment without it, the whole package fails —
a cost go-git would not impose.

## Alternatives considered

### 1. A pure-Go git library (go-git/go-git)

Embed go-git and call its status, diff, log, and clone APIs in-process, with
no dependency on a host `git` binary — mirroring the pure-Go choice
[ADR-003](003-privacy-tools-pure-go.md) made for privacy tools.

**Rejected because**: go-git's status / diff / rename-detection semantics
diverge from the user's CLI git — it does not honour `gitattributes`,
`core.autocrlf`, and `.gitignore` precedence identically — and the panel's
entire contract is **parity with what `git status` shows the operator**. A
panel that disagrees with the user's own `git status` is worse than no
panel. Separately, go-git would not inherit the host's credential helpers
and SSH configuration that `clone.go` relies on, forcing iterion to
reimplement authentication that shelling out gets for free.

## Consequences

- **The panel always agrees with the operator's own `git`.** Same binary,
  same config files, same gitattributes/autocrlf/ignore handling — no
  parity drift between what iterion shows and what `git status` shows.
- **Clone auth is free and correct.** Credential helpers and SSH keys work
  exactly as for a manual clone; iterion ships no clone-auth code to keep
  in sync with git's.
- **The host `git` binary is a hard dependency.** Every operation requires
  `git` on PATH; sandbox images and runners must provide it, and its
  absence fails the whole package.
- **Data crosses the boundary as text.** Every result is parsed from git's
  textual output, which is why robustness work like NUL framing
  ([ADR-024](024-nul-framed-parsing-for-adversarial-git-metadata.md)) and
  locale pinning
  ([ADR-023](023-c-locale-pinned-stderr-substring-error-classification.md))
  is needed — costs go-git's structured objects would avoid.
- **Re-challenge — no git binary available.** If iterion must run where no
  `git` binary is present (a minimal / locked-down sandbox image, WASM, or
  a fully in-process cloud runner), the host-binary dependency forces
  revisiting go-git for at least those environments.
