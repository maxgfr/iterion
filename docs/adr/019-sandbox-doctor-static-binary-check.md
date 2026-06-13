# ADR-019: sandbox doctor detects a dynamically-linked host iterion binary

- **Status**: Accepted
- **Date**: 2026-06-13
- **Authors**: devthejo
- **Code**: [pkg/cli/sandbox.go](../../pkg/cli/sandbox.go)
  (`detectBinaryLinkage`, `staticBinaryWarning`, `binaryLinkage`),
  [pkg/internal/proc/iterion_binary.go](../../pkg/internal/proc/iterion_binary.go)
  (`LocateIterionBinary`),
  [pkg/runtime/sandbox_mounts.go](../../pkg/runtime/sandbox_mounts.go)
  (`addClawBinaryMount` — the mount this check guards)

## Context

The host `iterion` binary is bind-mounted into sandbox containers at
`/usr/local/bin/iterion` (`addClawBinaryMount`) so the in-container
`iterion __claw-runner` subprocess can run. The default devbox build is
`CGO_ENABLED=1`, which dynamically links against **nix glibc**; that
binary runs fine on the host but cannot exec inside a container, because
the nix `ld-linux` loader the binary's `PT_INTERP` header points at does
not exist there. The failure surfaces only at runner-invocation time,
deep inside a sandboxed run, as the cryptic
`exec: /usr/local/bin/iterion: no such file or directory` — which reads
like a missing file, not a linkage problem (the file *is* mounted; its
loader is missing). CLAUDE.md already documents the fix (build
`CGO_ENABLED=0` / `task build`), but nothing detected the condition
ahead of a run.

`iterion sandbox doctor` is the natural pre-flight surface: it already
reports host/driver/runtime facts. We want it to warn when the resolved
host binary is dynamically linked.

## Decision

Add an in-process linkage probe to the basic doctor report:

1. Resolve the binary with the existing `proc.LocateIterionBinary()` so
   the doctor inspects the **same** binary the mount would pick (the
   helper mirrors the mount's resolution order).
2. `detectBinaryLinkage` opens the file with the standard library's
   `debug/elf` and classifies it by the presence of a **`PT_INTERP`
   program header** — present iff the binary defers to an external
   dynamic loader. Three outcomes: `static`, `dynamic`, `unknown`.
3. `staticBinaryWarning` emits an operator-facing WARNING **only** for
   `dynamic`, naming both the failure mode (`exec: … no such file or
   directory` from `__claw-runner`) and the fix (`CGO_ENABLED=0`).

### Why `debug/elf` PT_INTERP, not `ldd`/`file`

- **No subprocess, no host tooling assumptions.** `ldd`/`file` would
  require those binaries on PATH (not guaranteed) and `ldd` *executes*
  the target to resolve libraries — undesirable and itself subject to
  the loader being present. `debug/elf` is stdlib, parses the file
  statically, and reads exactly the one header (`PT_INTERP`) that
  governs whether exec will succeed. The signal is causal, not a proxy.
- **`PT_INTERP` over imported-library inspection.** A binary's
  `DT_NEEDED` list (imported shared libraries) is a weaker proxy; the
  loader path in `PT_INTERP` is precisely what the kernel resolves at
  exec time, so it is the most direct predictor of the in-container
  failure.

### Why `unknown` stays silent (the deliberate trade-off)

A non-ELF file (macOS Mach-O), an unreadable path, or an empty
resolution all yield `unknown`, which emits **no** warning. We
deliberately accept a **false negative off-Linux** (a non-ELF host that
somehow hit this path gets no warning) to guarantee **zero false
positives**: the dynamic-loader failure is Linux/ELF-specific, and a
spurious warning on macOS — where the host binary is never a nix-glibc
ELF — would be noise that erodes trust in the doctor. The check fires
only when the failure is positively detected on the platform where it
actually occurs.

## Consequences

**Positive**
- The mount's most common foot-gun (dynamic devbox build) is caught at
  `iterion sandbox doctor` time with an actionable message, instead of
  as an opaque exec error mid-run.
- Pure stdlib, no new dependency, no subprocess, no host-tool
  assumption; the JSON report gains two additive keys
  (`iterion_binary`, `iterion_binary_link`).

**Negative / limits**
- Off-Linux hosts get no signal (accepted, see above). If a cloud
  runner ever mounts a non-ELF host binary this would not warn — but
  that path is Linux-only by construction.
- The probe inspects the binary `LocateIterionBinary` resolves, which
  may differ from a binary a future mount override pins; the two share
  resolution order today, so they agree.

## Alternatives considered

1. **Shell out to `ldd`/`file`** — rejected: external tool dependency,
   and `ldd` executes the target (the very thing that fails in-container).
2. **Inspect `DT_NEEDED` imported libraries** — rejected: a weaker proxy
   than the `PT_INTERP` loader header for exec-time success.
3. **Warn on every `unknown`** — rejected: false positives on macOS
   would make the doctor cry wolf; we optimise for precision over recall
   on a Linux-specific failure.
4. **Enforce at mount time (hard error in the runtime)** — out of scope
   and heavier-handed; the doctor is the advisory pre-flight surface and
   the minimal, surgical place for this check.
