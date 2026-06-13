# ADR-020: Allowlist + filepath.IsLocal hardening of git arg/path surfaces

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: Adry
- **Code**: [pkg/git/safety.go](../../pkg/git/safety.go) (`ValidateRelPath`, `ValidateBranchName`, `branchNameAllowed`), [pkg/git/worktree_file.go](../../pkg/git/worktree_file.go) (`lstatWorktreePath`), [pkg/git/clone.go](../../pkg/git/clone.go) (`ShallowClone`)

## Context

`pkg/git` builds git command lines and worktree file paths from
user-controlled surfaces. Three are externally reachable:

- `--branch-name` — flows in from the CLI flag, the Launch API, and the
  studio Launch modal into the storage-branch name handed to `git branch`.
- the `?path` HTTP query — the studio file panel reads it and passes it to
  `os.ReadFile` and to `git show <ref>:<path>`.
- `ref:path` positionals — `showAt` ([pkg/git/range.go](../../pkg/git/range.go))
  concatenates a ref and a relative path into a single `git show` argument.

Two failure modes make the obvious validation insufficient. First, a value
beginning with `-` is re-parsed by git as a **flag**: `git show HEAD:-v`
turns on verbose mode and leaks unrelated output, and a leading-dash branch
name can smuggle options into `git branch`. Second, a naive containment
check (`filepath.Clean` + `strings.HasPrefix(workdir)`) is famously
bypassable, and following symlinks during the check lets a worktree symlink
point a "contained" path at an arbitrary host file.

## Decision

A tight allowlist regex gates branch names:
`^[A-Za-z0-9][A-Za-z0-9._/-]*$` in
[`pkg/git/safety.go`](../../pkg/git/safety.go) (`branchNameAllowed`),
deliberately stricter than git's own `check-ref-format` so every accepted
value also survives git's downstream check. The leading byte must be
alphanumeric, which alone forecloses the leading-dash flag-injection path;
`ValidateBranchName` layers on the remaining `check-ref-format` rules
(`..`, `//`, trailing `/`/`.`, `.lock`, 255-byte cap, NUL).

`ValidateRelPath` rejects absolute paths, NUL bytes, and a leading `-`
explicitly, then defers containment to `filepath.IsLocal` (Go 1.20+)
instead of prefix matching — `IsLocal` judges `..` segments, empty
segments, and drive letters by the OS rules. The input is normalised to OS
separators (`filepath.FromSlash`) only for that single check so the verdict
is identical on Windows and Linux.

`lstatWorktreePath` in
[`pkg/git/worktree_file.go`](../../pkg/git/worktree_file.go) walks the path
one component at a time and treats any **symlinked parent component** as
missing (`fs.ErrNotExist`); only the final component may be a symlink, and
it is read as link text rather than followed. A path therefore cannot
escape the worktree through a linked directory.

Callers pair these validators with a `--` sentinel before user values —
`ShallowClone` in [`pkg/git/clone.go`](../../pkg/git/clone.go) appends
`"--", url, dest`, so even a pathological URL or dest cannot be parsed as a
clone flag. The allowlist is defense-in-depth that fails earlier, with a
friendly error, rather than relying on `--` alone.

## Trade-offs

| Dimension | Chosen approach | Rejected approach |
|---|---|---|
| Containment check | `filepath.IsLocal` (OS-rule semantics) | `filepath.Clean` + `HasPrefix(workdir)` |
| Symlink handling | parent symlink ⇒ treated as missing | `os.Stat` follows links during the check |
| Flag injection | leading-`-` rejected + `--` sentinel | rely on git argument order alone |
| Branch names | allowlist stricter than `check-ref-format` | accept anything git's own check accepts |

The cost: the allowlist rejects some branch names git would technically
accept (e.g. names with `@`, `+`, or non-ASCII letters), so an operator
with an exotic naming scheme gets a validation error rather than the branch
they asked for.

## Alternatives considered

### 1. `filepath.Clean` + `HasPrefix(workdir)` traversal guard with symlink-following `os.Stat`

Validate by cleaning the path, joining it onto the working directory, and
checking the result still has the working directory as a prefix — resolving
symlinks with `os.Stat` along the way.

**Rejected because**: prefix checks against a cleaned path are a known-leaky
containment primitive, symlink-following lets a worktree symlink escape to
arbitrary host files, and neither catches the leading-`-` flag injection
through `git show ref:-flag` — a path that is perfectly "contained" can
still be re-parsed by git as an option.

## Consequences

- **Flag injection is closed at two layers.** The allowlist/leading-dash
  rejection stops it at validation time with a clear error; the `--`
  sentinel stops anything that slips past. Removing either leaves the other
  as a backstop.
- **Containment is delegated to the standard library.** `filepath.IsLocal`
  is maintained by the Go team and tracks OS edge cases (drive letters,
  reserved names) that a hand-rolled prefix check would miss.
- **Parent-symlink escapes are impossible; final-component symlinks are
  preserved.** The panel still renders a symlink's target text (git-like
  behaviour) without letting a linked parent directory redirect the read.
- **Re-challenge — multi-tenant network exposure.** If file contents are
  ever served to untrusted multi-tenant users over the network (cloud
  mode), reading the final-component symlink target may itself need locking
  down, since the link text can point outside the worktree even though the
  link node is inside it.
