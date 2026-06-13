# C082 board-emit fix — implementation plan

Status: **root-caused + fully designed; ready for a focused implementation pass.**
Branch: `c082-board-emit` (worktree off local HEAD).

## Symptom

Sandboxed `sec-audit-source` (Seki) `report_card` (backend `claude_code`) calls
`board.create` ×3, its output carries native-looking IDs (`native:90543c66…`),
but **nothing lands on the operator's board** (run 019ec230: board total stayed
94; fetch-by-id + every label query miss). The agent **confabulated** the IDs.

## Root cause (verified in code)

The sandboxed board MCP HTTP transport is **declared on both ends but the
producer side is never connected**:

- **Consumer exists**: [pkg/backend/delegate/claude_code.go:477](pkg/backend/delegate/claude_code.go)
  uses `task.BoardHTTPEndpoint` + `task.BoardRunToken`; :490 warns + **disables
  board MCP** when they're empty under a sandbox.
- **Server exists**: [pkg/server/mcp_board_handler.go](pkg/server/mcp_board_handler.go)
  (`BoardMCPTokenRegistry`, `RegisterBoardMCPRoutes`) + server.go:870 wires
  `/api/v1/mcp/board`.
- **Producer MISSING**: grep shows **nothing ever assigns
  `Task.BoardHTTPEndpoint`/`BoardRunToken`** and **nothing ever calls
  `boardMCPTokens.Register(token, caps)`** for a run. So claude_code always hits
  the :490 "board MCP disabled" path → no board tool → confabulation.

## Why not the simpler stdio-to-mounted-store route

Non-sandboxed claude_code uses the `__mcp-board` **stdio** server (writes the
board store directly). In a sandbox the store *is* visible (the workspace
`.iterion/dispatcher` is bind-mounted), so a stdio `__mcp-board` inside the
container could write it — BUT [pkg/dispatcher/native/store.go:40](pkg/dispatcher/native/store.go)
guards writes with only an **in-process `sync.Mutex`** (no flock). The studio
process and an in-container process are **different processes** → concurrent
`board.json` writes would corrupt it. The HTTP transport exists precisely to
**serialize all writes through the one studio process's Store**. So the fix must
stay HTTP.

## The networking constraint

`iterion studio` binds **127.0.0.1** (loopback). The container cannot reach
`127.0.0.1` (that's the container itself) and `host.docker.internal:4891` →
gateway IP → a 127.0.0.1-bound server isn't listening there. (The egress proxy
works only because the docker driver's `ProxyConfigurer` binds it
gateway-reachable — see [pkg/runtime/sandbox.go:422](pkg/runtime/sandbox.go)
`proxyAddressesForDriver` + :318 `startNetworkProxy`.) `NO_PROXY` also contains
`127.0.0.1`, so routing board calls through the proxy to loopback won't work
either. The board endpoint therefore needs its **own gateway-reachable bind**.

## Design

Start a **per-run gateway-reachable board MCP listener** alongside the egress
proxy in the sandbox-start path, serving the board routes against the **same
in-process `native.Store` instance** the studio uses (so the Store mutex
serializes container writes with studio writes — no corruption).

1. **Resolve the import cycle.** `pkg/runtime` cannot import `pkg/server`
   (server → runview → runtime). The board MCP HTTP handler + token registry
   currently live in `pkg/server`. Options:
   - **(preferred)** Relocate `RegisterBoardMCPRoutes` + `BoardMCPTokenRegistry`
     + the handler to a neutral package (e.g. `pkg/dispatcher/native/boardmcp`
     or extend `pkg/dispatcher/native/boardops`) that both `pkg/server` and
     `pkg/runtime` import. `pkg/server` keeps a thin wrapper for back-compat.
   - **(alt)** Pass a `StartBoardListener func(caps []string) (endpoint, token
     string, stop func(), err error)` **closure** from `pkg/server` into the
     runtime via `SandboxParams`; the closure (server-side) registers the token
     + serves the routes, but it still needs the driver's gateway bind from
     runtime → awkward. Relocation is cleaner.

2. **Plumb the store + token registry to the sandbox-start.** Add to
   [SandboxParams](pkg/runtime/sandbox.go) (~line 136): `BoardStore
   *native.Store` + `BoardTokens *boardmcp.TokenRegistry` (neutral types, no
   server import). The server populates them when constructing the
   Engine/executor for a studio run (it has `s.cfg.NativeTrackerStore` +
   `s.boardMCPTokens`). CLI runs leave them nil → behaviour unchanged (board
   caps stay disabled under sandbox; documented).

3. **Start the listener in `resolveAndStartSandbox`** (after the proxy, ~line
   239), only when `p.BoardStore != nil` AND the workflow has any board-cap
   node AND the driver is real (not noop). Bind gateway-reachable via the same
   `proxyAddressesForDriver(driver)` mechanism the proxy uses (`127.0.0.1:0`
   bind + `host.docker.internal` advertise on docker). Serve
   `RegisterBoardMCPRoutes(mux, "/", BoardStore, BoardTokens)`. Return its
   advertised endpoint + a `stop func()` (shut it down in the sandbox cleanup
   alongside the proxy).

4. **Register a per-run token + set the Task fields.** The executor, when
   building a Task for a node with board caps under a sandbox
   ([executor.go ~1245](pkg/backend/model/executor.go)), calls
   `BoardTokens.Register(token, caps)` and sets `task.BoardHTTPEndpoint =
   <listener endpoint>/` + `task.BoardRunToken = token`. The endpoint must reach
   the executor — thread it from the active sandbox (the Engine already holds
   the `activeSandbox`; expose its board endpoint to the executor like it does
   `e.sandbox`).

## Validation (must be live — container reachability)

- Build the worktree binary (`CGO_ENABLED=0`).
- Run a **dedicated** `iterion server --store-dir <tmp> --port <P>` from the
  worktree binary, bound so the docker gateway can reach it (the per-run
  listener handles this; the main server bind is irrelevant to the listener).
- Launch Seki sandboxed against it (`enable_deepsec=false`, `remediate=false`)
  via its API; on `report_card`, assert the board **total increases** and the
  created issue is fetchable by id/label. This proves the container reached the
  gateway listener and the write serialized into the shared Store.
- Negative check: confirm CLI runs (no server) still disable board caps under
  sandbox cleanly (no confabulation regression — ideally the agent gets a tool
  that errors honestly rather than a missing tool it confabulates around;
  consider surfacing the :490 disable as a hard "board unavailable" tool result).

## Files touched (estimate)

- `pkg/dispatcher/native/boardmcp/` (new, relocated handler+registry) or
  `pkg/dispatcher/native/boardops`.
- `pkg/server/mcp_board_handler.go` + `server.go` (thin wrapper / populate
  SandboxParams).
- `pkg/runtime/sandbox.go` (SandboxParams fields + listener start/stop) + the
  Engine→executor board-endpoint exposure.
- `pkg/backend/model/executor.go` (Task builder: register token + set fields).
- Tests: a unit test for the token-register+Task-field wiring; the live
  sandboxed run for end-to-end.

## Interim mitigation (optional, cheap)

Until the listener lands, make the confabulation **honest**: when board caps are
granted under a sandbox but `BoardHTTPEndpoint` is empty, claude_code should
expose a board tool that returns an explicit "board unavailable in this run"
error rather than no tool (the agent then can't confabulate created IDs). This
turns a silent data-loss into a visible degradation.

## Validation findings (2026-06-14) — implemented; one layer remains in claude-code

Implemented on this branch (parts 1–4) and live-validated each iterion-side layer
in isolation via a dedicated studio (worktree binary, port 4899, isolated store)
running a minimal sandboxed claude_code board.create bot:

1. **Gateway listener** — starts per run: `board MCP listener on
   http://host.docker.internal:<port>/api/v1/mcp/board`. ✅
2. **Producer wiring** — claude is exec'd with the correct inline
   `--mcp-config {"mcpServers":{"iterion_board":{"type":"http","url":...,
   "headers":{"X-Iterion-Run":...}}}}`. ✅
3. **Inline MCP config** (part 3) — fixed `MCP config file not found` (host /tmp
   file invisible to the container); config now passed as an inline JSON string. ✅
4. **NO_PROXY + proxy host.docker.internal→loopback** (parts 3–4) — a container
   reaches a host `0.0.0.0` listener via `host.docker.internal` directly (verified
   with a controlled `docker run` curl). ✅
5. **Handler MCP protocol** — `initialize` + `tools/list` return correct responses
   with `Content-Type: application/json` (verified via curl against the live
   endpoint with a real token). ✅

**Remaining gap (claude-code's own MCP client, NOT iterion):** despite a reachable,
protocol-correct endpoint, the sandboxed `claude_code` run reports `iterion_board`
is "not registered" / never appears among connected MCP servers (only the operator's
user-config servers — claude.ai Gmail/Calendar/Drive — connect). No MCP connect
error is surfaced to stderr. So claude-code's **Streamable-HTTP MCP client** is not
completing the handshake against our endpoint. Likely suspects to debug with
claude-code's internal MCP logging (e.g. `--mcp-debug` / verbose):
- It may require an `Mcp-Session-Id` header on the `initialize` response (the
  handler is stateless/token-based and returns none).
- It may require SSE (`text/event-stream`) or a GET endpoint for the stream, not
  just single `application/json` POST responses.
- Protocol-version negotiation mismatch (client sends a newer `protocolVersion`
  than the handler's `2024-11-05`).
- Its proxy-agent handling of the MCP URL.

**Net:** the entire iterion producer side (the documented root cause — "producer
never wired") is fixed + verified. Board-emit end-to-end is one focused
claude-code-MCP-client step away: enable claude-code MCP debug, capture the
`iterion_board` connect attempt, and align the handler's Streamable-HTTP behavior
(session-id / SSE / protocol-version) to what the client requires.
