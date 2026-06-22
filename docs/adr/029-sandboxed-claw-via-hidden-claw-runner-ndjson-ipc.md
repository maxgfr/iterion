# ADR-029: Sandboxed claw via hidden claw-runner NDJSON IPC

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [cmd/iterion/claw_runner.go](../../cmd/iterion/claw_runner.go), [pkg/backend/delegate/io.go](../../pkg/backend/delegate/io.go), [pkg/backend/delegate/multiplexer.go](../../pkg/backend/delegate/multiplexer.go)

## Context

Sandboxed claw calls need two properties that pull in opposite directions. Filesystem-affecting built-in tools must run inside the sandbox container so reads, writes, shell commands, and edits see the isolated run worktree rather than the launcher's host working directory. At the same time, MCP servers, custom engine-side tools, user prompts, and observability hooks often exist only in the launcher process.

A single local/remote placement for all tools cannot satisfy both constraints. The code therefore treats the sandbox boundary as a process boundary with an explicit typed protocol rather than trying to smuggle launcher closures into the container.

## Decision

Iterion runs sandboxed claw work through a hidden `iterion __claw-runner` subcommand inside the container. The runner is declared in [`cmd/iterion/claw_runner.go`](../../cmd/iterion/claw_runner.go) and exchanges bidirectional NDJSON envelopes over stdin/stdout.

The launcher sends one task envelope described by [`pkg/backend/delegate/io.go`](../../pkg/backend/delegate/io.go). The runner can emit intermediate envelopes for tool calls, ask-user requests, session capture, and events, and it finishes with one result envelope.

Inside the runner, built-in tools such as `bash`, `read_file`, `glob`, `grep`, `file_edit`, `web_fetch`, and `write_file` are rebuilt to execute locally in the container workdir. MCP tools, custom engine-side tools, and other launcher-only capabilities are represented as proxy tool definitions; their executions become envelope tool calls that the launcher-side multiplexer in [`pkg/backend/delegate/multiplexer.go`](../../pkg/backend/delegate/multiplexer.go) dispatches and correlates with tool-result envelopes.

## Trade-offs

| Dimension | Chosen hybrid runner | Pure launcher-side IPC | Pure in-container execution |
|---|---|---|---|
| Filesystem isolation | Built-ins mutate only the sandbox worktree. | Built-ins would run from the launcher process and can touch host state. | Built-ins are isolated. |
| Host-only tools | MCP/custom tools proxy to the launcher. | Host-only tools work naturally. | Host-only MCP servers and engine closures are unreachable. |
| Protocol complexity | Requires NDJSON envelopes, correlation IDs, and a multiplexer. | Simpler process placement. | Simpler tool placement. |

The honest concession is that the hybrid split makes tool placement an architectural category, not a mere implementation detail.

## Alternatives considered

### 1. Run every tool through launcher-side IPC

The runner could have proxied every tool call to the launcher and kept tool execution in one process.

**Rejected because**: that breaks sandbox filesystem isolation; `bash`, file readers, and editors would operate in the launcher's host cwd rather than the container worktree.

### 2. Run every tool locally inside the container

The runner could have attempted to instantiate all tools inside the container and avoid host callbacks.

**Rejected because**: MCP managers, custom engine tool closures, and user-interaction plumbing are launcher-side state and cannot be reconstructed inside the sandbox process.

## Consequences

- **Sandboxed built-ins are genuinely isolated.** File and shell operations execute where the run worktree is mounted, so tool side effects do not escape to the operator's host cwd.
- **Launcher-only capabilities remain available.** MCP/custom tools continue to work because the multiplexer bridges their calls back to the process that owns them.
- **The IPC protocol is now a compatibility seam.** Envelope types and correlation semantics must remain stable across runner and launcher code.
- **Observability crosses the same channel.** Session capture and events are modelled as envelopes rather than Go callbacks, which keeps closure state out of the sandbox.
- **Rechallenge if a third placement category appears.** Tools requiring simultaneous trusted host state and container-local filesystem state may need a third category beyond local built-ins and launcher-proxied tools.
