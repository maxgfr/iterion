# ADR-006: `iterion sandbox doctor --strict` — dry-run validation that never pulls

- **Status**: Accepted
- **Date**: 2026-05-28
- **Authors**: devthejo
- **Code**: [pkg/cli/sandbox_strict.go](../../pkg/cli/sandbox_strict.go)
  (`buildStrictReport`, `PreflightSandbox`),
  [pkg/runtime/sandbox.go](../../pkg/runtime/sandbox.go)
  (`ResolveSandboxSpecForDoctor`, `ResolveNetworkPolicy`),
  [pkg/sandbox/docker/probe.go](../../pkg/sandbox/docker/probe.go)
  (`PingDaemon`, `ResolveImageRef`, `ValidateSpecMounts`),
  [pkg/sandbox/kubernetes/validate.go](../../pkg/sandbox/kubernetes/validate.go)
  (`ValidateSpec`, `PingContext`)
- **Docs**: [docs/sandbox.md §Strict pre-flight](../sandbox.md)

## Context

Some sandbox misconfigurations only surfaced ~30s into a run with a
cryptic Docker/Kubernetes error: an image tag that does not exist, a
stopped Docker daemon, `host_state: auto` on the cloud (kubernetes)
driver, a malformed network-allowlist rule. The engine validates the
spec lazily — `Driver.Prepare` / `Driver.Start` fail at run time, after
the worktree, store, and executor are already wired.

We wanted a `--strict` doctor that resolves the *exact* spec a run would
use (host + optional workflow file + the same `--sandbox` /
`--sandbox-default-image` / `--sandbox-host-state` flags) and validates
every combination in ~1s, with a per-failure remediation hint and a
non-zero exit, plus an opt-in pre-flight hook so `iterion run` can fail
fast.

Three tensions shaped the design:

1. **"No actual pull."** The acceptance criterion is explicit: check
   image *resolvability*, not pull-ability. But `docker.Prepare` — the
   obvious place to get image validation "for free" — pulls the image
   when it is missing locally.
2. **Layering.** Spec resolution (mode + `host_state` precedence,
   default-image fallback, devcontainer reading) lives in `pkg/runtime`,
   which imports `pkg/sandbox`. The natural home for a reusable checker
   would be `pkg/sandbox`, but it cannot import `pkg/runtime` (cycle).
3. **The kubernetes driver is in-cluster-only.** `kubernetes.Detect`
   requires a service-account mount, so the factory never selects it on
   a developer laptop — yet the most valuable strict check is "will my
   workflow run in the cloud?", which must work *off* the cluster.

## Decision

**1. Image resolvability uses `docker manifest inspect`, never
`Prepare`.** `docker.ResolveImageRef` first short-circuits when the image
is already in the local cache (it will run), otherwise reads only the
registry manifest — no blob download. A genuine "manifest unknown / not
found" is a **fail**; an auth/network error is a **transient** warning
(`*ImageResolveError.Transient`) because we cannot prove resolvability
offline and the daemon may still pull with its own credentials. We do
**not** call `docker.Prepare` from the doctor.

**2. Probes live in the driver packages; the dry-run spec resolver is
exported from `pkg/runtime`; orchestration stays in `pkg/cli`.** Driver
packages own runtime detection and the `LC_ALL=C` exec wrappers, so the
new `PingDaemon` / `ResolveImageRef` / `ValidateSpecMounts` (docker) and
`ValidateSpec` / `PingContext` (kubernetes) belong there and are unit
tested with the existing exec-indirection seam. `pkg/runtime` gains two
thin exports — `ResolveSandboxSpecForDoctor` (resolve + bake `host_state`
into the spec, **no** mounts / pull / `os.Stat`) and `ResolveNetworkPolicy`
(rename of the internal derivation the proxy already uses). `pkg/cli`
(which already imports runtime, sandbox, the drivers, and netproxy)
composes them. No new package, no import cycle.

**3. `kubernetes.ValidateSpec` is pure and host-independent; the live
context probe is conditional.** The side-effect-free cloud constraints
(no `build:`, image required, `host_state != auto`, numeric user) are
factored out of `Driver.Prepare` into `ValidateSpec`, which `Prepare` now
calls. `--target cloud` runs that battery from any host. The live
`PingContext` probe is **fatal** only when the *selected* driver is
kubernetes (in-cluster / `ITERION_MODE=cloud`); off-cluster it is
**advisory** (this host is not a runner).

**4. Exit codes split config from usage.** A failed strict check returns
a plain error → exit 1 (the host/spec is misconfigured). A bad
file/flag (unparseable workflow, unknown mode) returns `UserInputError`
→ exit 2. Warnings never change the exit code.

**5. The pre-flight hook is opt-in via `ITERION_SANDBOX_PREFLIGHT`.** It
reuses `buildStrictReport` through `PreflightSandbox`, logging warnings
and failures and aborting the run (exit 2) on any failure. Default off.

## Trade-offs

| Dimension | Chosen | Rejected alternative |
|---|---|---|
| Image check | `manifest inspect` + local-cache short-circuit (no pull) | `Driver.Prepare` (validates *and pulls* — violates "no pull", slow, mutates the host image cache) |
| Reusable checker home | probes in drivers, resolver in runtime, orchestration in cli | a `pkg/sandbox/doctor` package (can't reach the runtime resolver without a cycle, or must duplicate the precedence chains and drift) |
| Auth/network image error | transient → **warn** | treat as fail (false negatives for private registries the daemon can still pull) |
| Cloud-compat off-cluster | pure `ValidateSpec` via `--target cloud`; live probe advisory | only validate when the k8s driver is selected (impossible on a laptop — the most useful case is unreachable) |
| Pre-flight | opt-in env, default off | on by default (adds a daemon/registry round-trip — real latency — to every run) |
| Strict-fail exit | 1 (config) vs 2 (usage) | a single code (loses the script-branchable distinction the CLI already draws with `ErrUserInput`) |

The honest concessions: (a) the image check cannot *guarantee* a pull
will succeed — registry credentials applied daemon-side at pull time are
invisible to a `manifest inspect`, hence the transient-warn bucket; and
(b) the off-cluster k8s context probe is advisory, so a laptop run of
`--target cloud` validates *spec* compatibility but not *cluster*
reachability (which only the in-cluster runner can verify); and (c) the
"driver available" check is likewise advisory (warn, not fail) under an
explicit cross-host `--target` — see the 2026-05-31 update below.

## Alternatives considered

### Call each driver's `Prepare` and surface its error

Reuse the drivers' own validation by dry-running `Prepare`.

**Rejected for docker**: `docker.Prepare` pulls a missing image — the
exact side effect the feature forbids, plus minutes of latency and a
mutated host image cache. **Adopted for kubernetes** only after
extracting the pure `ValidateSpec` (its `Prepare` has no IO), so the
doctor gets the cloud constraints verbatim with no pod created.

### A standalone `pkg/sandbox/doctor` package

Put the whole battery in one cohesive package.

**Rejected**: it would need the runtime's spec-resolution + `host_state`
precedence, which lives in `pkg/runtime` and cannot be imported from
under `pkg/sandbox` without a cycle. The options were to duplicate the
precedence logic (drift risk against the engine's actual behaviour) or
invert the dependency (larger change). Keeping orchestration in `pkg/cli`
— which legitimately imports both layers — was the smaller, drift-free
move.

### Pre-flight on by default

Run the strict battery before every `iterion run`.

**Rejected**: the battery shells out to the Docker daemon and an image
registry. Paying that on every run (including fast iteration loops and
recipe runs with no sandbox) is a regression; the env-gated opt-in keeps
the fast path fast while making the safety net available where it earns
its latency (CI, first run of a dispatcher session).

## Consequences

- **`iterion sandbox doctor --strict [file]`** validates driver
  availability, Docker daemon liveness, image resolvability (no pull),
  mount safety, kubernetes cloud-compatibility + context, the
  `host_state`-vs-k8s mutual exclusion, and network-allowlist syntax —
  each with a remediation hint — and exits non-zero on any failure.
- **`--target cloud`** lets an operator validate a cloud workflow's spec
  from a laptop without a cluster.
- **`pkg/runtime` gains two stable exports** (`ResolveSandboxSpecForDoctor`,
  `ResolveNetworkPolicy`); the internal `resolveNetworkPolicy` caller was
  updated in lockstep.
- **`kubernetes.Prepare` now delegates to `ValidateSpec`** — one source of
  truth for the cloud constraints, exercised by both the driver and the
  doctor.
- **`ITERION_SANDBOX_PREFLIGHT`** opts `iterion run` into the same battery
  before engine boot; `ITERION_SANDBOX_DOCTOR_TIMEOUT` caps each probe
  (default 5s). The dispatcher pre-flight (one `sync.Once` per daemon
  session) is a noted follow-up reusing `PreflightSandbox`.

## Update — 2026-05-31: cross-host `--target` no longer fails on local driver availability

A review follow-up found that `iterion sandbox doctor --strict --target
cloud` (and `--target local`) hard-failed the **"driver available"**
check — exit 1 — on a host whose local runtime cannot serve the forced
battery (most acutely a *runtime-less* host: no Docker/Podman, where
`Factory.DriverForSpec` refuses to degrade to noop on an active spec).
That conflated *local runtime availability* with *cross-host spec
validation* and defeated this ADR's headline use case ("validate a cloud
spec from a laptop"): the report content was correct, yet the exit code
was 1 for a perfectly valid spec.

**Decision.** When an explicit `--target` selects a host class the
locally-selected driver does not naturally serve — or this host has no
driver at all — the "driver available" check is downgraded from `fail`
to `warn` (`crossHostDoctorValidation` in
[pkg/cli/sandbox_strict.go](../../pkg/cli/sandbox_strict.go)). A valid
cross-host spec now exits 0; the warning still records that the spec is
not runnable *here*.

**Trade-off.** The alternative — keep the hard fail — preserves a single
crisp signal ("this host cannot run the sandbox") but makes the
documented off-cluster validation path unusable, since exit 1 is
indistinguishable (to a script) from a genuinely broken spec. We
deliberately gate the downgrade on an **explicit** `--target`: a plain
`--strict` (no target / `auto`) on a runtime-less host still fails, so
the "I meant to run this locally and can't" signal is retained. The
residual concession: the symmetric live-probe checks for the *forced*
battery (`docker daemon` under `--target local`, `k8s context` under
`--target cloud`) can still surface as fail/warn for a foreign host —
`k8s context` already self-downgrades when the selected driver is not
kubernetes; a `docker daemon` equivalent under cross-host `--target
local` is a noted follow-up, out of scope for this change.
