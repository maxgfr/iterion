# ADR-031: Sandbox uses one long-lived sleep-infinity container per run

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/sandbox/docker/driver.go](../../pkg/sandbox/docker/driver.go), [pkg/sandbox/kubernetes/manifest.go](../../pkg/sandbox/kubernetes/manifest.go)

## Context

A single sandboxed run can issue many delegate executions and tool calls. Container or pod creation, image pulls, readiness, mount setup, and environment setup are expensive compared with an individual `exec` into an already-running workload.

The sandbox also needs stable per-run state: a mounted workspace, injected proxy and CA settings, post-create results, and a consistent working directory for repeated tool invocations.

## Decision

The Docker driver starts one container per run whose PID 1 is `sleep infinity` in [`pkg/sandbox/docker/driver.go`](../../pkg/sandbox/docker/driver.go). Subsequent commands enter that container with `docker exec`.

The Kubernetes manifest mirrors this model in [`pkg/sandbox/kubernetes/manifest.go`](../../pkg/sandbox/kubernetes/manifest.go): the workload container command is `sleep infinity`, and the engine repeatedly uses `kubectl exec` into the same pod for delegate invocations.

The image's own CMD/ENTRYPOINT is deliberately not used as the run process. The container is treated as a long-lived, ssh-like execution target for the duration of one iterion run.

## Trade-offs

| Dimension | One long-lived container per run | Fresh container per node/tool invocation |
|---|---|---|
| Latency | Amortises create/pull/readiness cost across all execs. | Pays startup cost for every invocation. |
| Per-run state | Preserves workspace and setup across calls. | Requires reconstructing state or re-mounting every time. |
| Isolation granularity | Isolation is per run. | Isolation can be per node or per tool call. |
| Cleanup reliance | Requires reliable end-of-run cleanup. | Short-lived containers naturally drop state sooner. |

The honest concession is that the chosen model favours run-level performance over node-level isolation.

## Alternatives considered

### 1. Start a fresh container for each node or tool call

Each delegate invocation could have used a new container or pod and exited when the command finished.

**Rejected because**: sandboxed runs issue many exec/tool calls, and repeated image/container startup would make common runs unacceptably slow.

### 2. Run the image's default entrypoint as PID 1

The sandbox could have started the image normally and attempted to exec into it while its application process stayed alive.

**Rejected because**: arbitrary image entrypoints may exit, block, mutate state, or shadow the intended "long-lived execution target" model.

## Consequences

- **Exec-heavy workflows are fast enough.** Startup and pull costs are amortised over the run rather than charged to every tool invocation.
- **Workspace state is stable across tools.** Repeated commands see the same mounts, working directory, environment, and post-create effects.
- **Cleanup matters more.** A leaked run container can keep state alive longer than intended, so lifecycle cleanup remains part of the correctness story.
- **The model is consistent across Docker and Kubernetes.** Both runtimes present the same per-run exec target to the engine.
- **Rechallenge if isolation needs change.** If node-level isolation or cleanup reliability becomes more important than latency, the per-run container model should be revisited.
