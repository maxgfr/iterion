# ADR-030: Sandbox network proxy binds all interfaces, not loopback

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/sandbox/docker/driver.go](../../pkg/sandbox/docker/driver.go), [pkg/sandbox/iface.go](../../pkg/sandbox/iface.go)

## Context

The sandbox network CONNECT proxy is started by the launcher but must be reachable from sibling sandbox containers. On Linux Docker, `host.docker.internal` is wired to the host-gateway bridge address, not to the host loopback interface.

Binding the proxy to `127.0.0.1` therefore creates a misleading configuration: the container receives a usable-looking proxy URL, but its connections arrive on the bridge IP and get refused because the listener is only on loopback.

## Decision

The Docker sandbox driver implements the optional proxy configuration hook in [`pkg/sandbox/docker/driver.go`](../../pkg/sandbox/docker/driver.go) and returns bind address `0.0.0.0:0` with advertised host `host.docker.internal`.

The common sandbox interface in [`pkg/sandbox/iface.go`](../../pkg/sandbox/iface.go) makes this explicit through `ProxyConfigurer`: drivers may choose an all-interface bind such as `0.0.0.0:0` or a loopback bind such as `127.0.0.1:0`, and separately choose the hostname injected into `HTTPS_PROXY`/`HTTP_PROXY` for containers.

The broader exposure from binding all interfaces is controlled by the per-run bearer token embedded in the proxy configuration. The listener being reachable is not sufficient to use it; requests still need the run-specific authorization material.

## Trade-offs

| Dimension | Bind `0.0.0.0:0` and advertise `host.docker.internal` | Bind `127.0.0.1:0` |
|---|---|---|
| Docker sibling reachability | Works because bridge-IP traffic reaches the listener. | Fails because host-gateway does not resolve to loopback. |
| Exposure surface | Listener is reachable on non-loopback interfaces but token-gated. | Listener is loopback-only. |
| Runtime specificity | Encodes Docker host-gateway behaviour in the Docker driver. | Assumes loopback behaviour Docker does not provide. |

The honest concession is that this relies on bearer-token auth to make an intentionally reachable listener safe.

## Alternatives considered

### 1. Keep the default loopback bind

The engine could have kept binding the proxy on `127.0.0.1:0` and still advertised `host.docker.internal` to the container.

**Rejected because**: Docker's host-gateway resolves to the bridge IP, not host loopback, so sandbox containers cannot connect to a loopback-only listener through that alias.

### 2. Advertise a raw host bridge IP

The driver could have discovered and advertised the bridge IP directly instead of the canonical Docker alias.

**Rejected because**: bridge discovery is runtime/platform-specific, while Docker already provides the stable `host.docker.internal:host-gateway` mechanism the driver wires with `--add-host`.

## Consequences

- **Docker sandbox proxying works from containers.** The injected proxy URL points at an address the sibling container can actually reach.
- **Authorization becomes the safety boundary.** The all-interface bind is acceptable only because every proxy request is protected by the per-run bearer token.
- **The interface keeps runtime choices local.** Other drivers can use a different bind/advertise pair without changing engine code.
- **Loopback assumptions are documented as invalid for Docker.** Future maintainers should not regress the bind address while keeping `host.docker.internal`.
- **Rechallenge if runtime behaviour changes.** If Docker host-gateway starts resolving to loopback, or supported runtimes provide a safer same-host channel, the bind strategy should be revisited.
