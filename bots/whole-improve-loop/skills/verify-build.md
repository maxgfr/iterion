---
name: verify-build
description: Detect and run a repository's OWN build + test so a deterministic gate can confirm the tree actually compiles and passes before the improve-loop commits. Stack-agnostic — read this when asked to verify a build.
---

# verify-build

Your job: make the repository in front of you **build and test green using its
own tooling**, and leave a re-runnable script for the deterministic gate.

This is the backstop for the whole-improve-loop review. Reviewers read **one
chunk of source at a time**, so a change that breaks compilation in *another*
file (a renamed or retyped symbol with a caller the chunk reviewer never saw)
or a bug only a test catches is invisible to them. The build + tests are what
catch it — that is the entire reason this step exists.

## 1. Detect the repo's build + test commands

Do **not** assume a language. Look at the markers actually present and prefer
the project's own wrapper when it has one — a wrapper encodes the correct flags
and the correct toolchain:

- **Task runner / Makefile** — `Taskfile.yml` → `task build` / `task test` /
  `task check`; `Makefile` → `make build` / `make test`; `Justfile` → `just …`.
  This is almost always the right answer when present.
- **Pinned toolchain — honour it or the build fails on a version mismatch.**
  `devbox.json` → prefix with `devbox run -- …`. `.tool-versions` (asdf/mise),
  `.nvmrc`, `flake.nix`, `mise.toml` → activate accordingly. For Go: if
  `go.mod`'s `go` directive is newer than the installed `go version`, use the
  project's wrapper (e.g. `devbox run -- go …`) or `GOTOOLCHAIN=auto go …` so
  the pinned toolchain is fetched, rather than failing on "requires go >= X".
- **Language defaults (only when there is no wrapper):**
  - Go (`go.mod`): `go build ./... && go test ./...`
  - Node (`package.json`): pick the package manager from the lockfile
    (`pnpm-lock.yaml`→pnpm, `yarn.lock`→yarn, `package-lock.json`→npm) and run
    its `build` + `test` scripts if defined.
  - Rust (`Cargo.toml`): `cargo build && cargo test`.
  - Python (`pyproject.toml`/`setup.py`): the configured test runner, e.g.
    `python -m pytest`, plus a type/lint check if the project defines one.
  - Anything else: build + unit-test the way the repo's CI does — read
    `.github/workflows/*` (or other CI config) if present; CI is the source of
    truth for "how this repo is built".

Prefer the **fast** path: compile the whole module (a compile error is the
common breakage) + run the unit tests. Skip slow integration / e2e / live
suites unless they are the only tests the repo has.

## 2. Write `.whole_improve_loop.verify.sh`

Write an executable POSIX-sh script at the **workspace root** that runs the
build AND tests you settled on and **exits non-zero on any failure**. Example
*shape* (adapt to the repo — illustration, not a fixed command):

```sh
#!/bin/sh
set -e
devbox run -- task build
devbox run -- task test
```

The deterministic gate re-runs **this** script and gates the commit on its real
exit code — so it must genuinely pass, not merely look plausible.

## 3. Run it, and fix what the just-applied changes broke

Run your script. If it fails, the failure is almost always introduced by the
changes the review loop just applied. Read the error and fix it **at the
source** with the **smallest** change that restores build + tests: update the
missed caller, correct the signature, fix the read/write mode, repair the test.
Do **not** refactor or change behaviour beyond what is needed to compile and
pass. Re-run until green.

If the repository genuinely has no build/test system (pure docs/config), write
a script that echoes that fact and exits 0, and say so plainly in your summary —
do not invent a build.
