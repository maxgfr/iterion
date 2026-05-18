# ADR-002: Desktop studio hosted via Wails AssetServer handler

- **Status**: Accepted
- **Date**: 2026-05-05
- **Authors**: feature-dev
- **Scope**: `cmd/iterion-desktop/`, `pkg/server/`, `studio/`

## Context

The desktop binary (`iterion-desktop`) is a thin native shell around the
existing CLI studio server (`pkg/server`). Its value proposition lives in
the **Wails IPC bindings** (`window.go.main.App.*`): project switcher,
secrets/keychain, CLI detection, auto-update — none of which the studio
SPA can do on its own.

The original design (Phase 1 of the desktop plan) loaded a tiny embedded
"bootstrap stub" in the Wails AssetServer, called `GetServerURL` /
`GetSessionToken` on `window.go.main.App`, and **navigated the WebView**
to the embedded loopback HTTP server so the studio SPA loaded from the
server directly.

That design has a fatal architectural flaw discovered in iteration 10 of
the desktop feature review:

1. Wails injects `/wails/runtime.js` and `/wails/ipc.js` — the scripts that
   define `window.go`, `window.runtime`, `window.WailsInvoke` — **only into
   HTML responses served by the AssetServer itself**
   (`vendor/.../assetserver.go:200-220`).
2. Custom URL schemes (`wails://`) and the `wails.localhost` host are the
   only origins the AssetServer intercepts; everything else, including
   `http://127.0.0.1:<random>`, is passed through to the WebView's default
   network stack (`vendor/.../windows/frontend.go:652-658`).
3. `BindingsAllowedOrigins` was unset, so the message dispatcher
   (`vendor/.../originvalidator/originValidator.go:46-58`) would have
   blocked any binding call from the localhost origin even if the runtime
   had somehow loaded.
4. Concretely: after the bootstrap-stub redirected the window to
   `http://127.0.0.1:<port>/`, `window.go` was `undefined`, `isDesktop()`
   returned `false`, and the SPA quietly fell back to browser mode — every
   desktop-only view (`<Welcome>`, `<Settings>`, `<ProjectSwitcher>`,
   `<MissingCLIBanner>`) was un-mounted, every native-menu emit silently
   discarded, every `desktop.*` call rejected with "Not available in
   browser mode".

Tests missed this because `studio/src/__tests__/desktopBridge.test.ts`
stubs `window.go` directly, and `bindings_project_test.go` uses a fake
`recordingServer` plus a stubbed `windowReloader`. Neither exercises the
real cross-origin runtime injection that Wails actually performs at
runtime.

## Decision

The Wails AssetServer hosts the studio SPA through a custom
`http.Handler` (`cmd/iterion-desktop/asset_proxy.go`). The WebView's main
origin stays on the AssetServer URL (`wails://wails/` on Mac/Linux,
`http://wails.localhost/` on Windows) for the lifetime of the app, and:

- The AssetServer handler serves SPA/static responses — every path outside
  `/api/*`, including `GET /` and hashed chunks under `/assets/*` — from
  the GUI binary's own `pkg/server.StaticFS` embed. Wails detects the
  handler's `text/html` response on `GET /` and injects
  `/wails/runtime.js` + `/wails/ipc.js`, which is what makes
  `window.go.main.App.*` reachable to the studio SPA — the exact gap that
  was the production blocker.
- Only `/api/*` HTTP traffic is forwarded to the selected loopback server
  through `httputil.ReverseProxy`. The proxy's `Rewrite` calls `SetURL`,
  forces `Host` to the loopback target, and rewrites any inbound `Origin`
  header to `http://<targetHost>`. That keeps `pkg/server`'s loopback
  Origin checks and CORS reflection satisfied without teaching HTTP
  handlers about the `wails://` origin scheme. The proxy does not attach
  or strip desktop auth cookies.
- Default GUI startup uses per-project headless daemons: it finds or
  launches `iterion-desktop --server-only --project=<dir>` and points the
  `/api/*` proxy plus direct WS URLs at that daemon. The in-process
  `cmd/iterion-desktop/server_host.go` path is the opt-out/fallback used
  when `ITERION_DESKTOP_ATTACH_DAEMON=0` (also `false`, `no`, or `off`) is
  set or daemon attach/spawn fails; that fallback starts `cli.RunStudio`
  with `Port=-1`, `Bind="127.0.0.1"`, and `NoBrowser=true`.
- Both the default headless daemon and the in-process fallback use
  `cli.RunStudio`, which builds `pkg/server.Config` with
  `DisableAuth=true`, the same local-editor trust model used by
  `iterion studio`; there is no desktop-specific `SessionToken` field in
  `pkg/server.Config`.
- `GetSessionToken` remains a Wails binding because the SPA's desktop WS
  helper calls it together with `GetServerURL`. In current local desktop
  builds it returns `""`, and the SPA omits the token query parameter when
  the value is empty.
- WebSocket upgrades (`/api/ws`, `/api/ws/runs/*`) **cannot** flow through
  the proxy: Wails' AssetServer rejects WS upgrades with `501` by design
  (`vendor/.../assetserver.go:110-114`). The studio dials WS endpoints
  directly at `ws://127.0.0.1:<port>/api/ws...` using the absolute URL
  resolved through `getDesktopWsBase()` in
  `studio/src/lib/desktopBridge.ts`. In local desktop mode those dials have
  no token query parameter; if a future hosted-auth desktop flow supplies a
  real token, the auth implementation that consumes it should document that
  conditional behaviour.

The bootstrap-stub directory (`cmd/iterion-desktop/frontend-stub/`) and
the `embed.go` that wired it up are deleted — they're obsolete with the
proxy approach.

## Alternatives considered

### 1. Inject Wails runtime + IPC scripts via the local server

Embed `runtime_prod_desktop.js` and `ipc.js` from `vendor/.../runtime/`
into the desktop binary, serve them at `/wails/runtime.js` +
`/wails/ipc.js` from `pkg/server`, and add `<script>` tags to
`studio/index.html`.

**Rejected**: the runtime needs the `window.wailsbindings` JSON to
populate `window.go.<package>.<class>.<method>`. That JSON is generated by
Wails internals during reflection on the `Bind` list; it's not exposed
through any public API. Without it, `window.go` would be `{}` even with
the runtime loaded. Reverse-engineering the bindings emitter into the
desktop binary would re-implement substantial Wails machinery and have to
track every Wails bump.

### 2. Move all desktop functionality to same-origin `/api/*` endpoints

Refactor every Wails binding (`ListProjects`, `SetSecret`,
`DetectExternalCLIs`, `CheckForUpdate`, …) into HTTP handlers on
`pkg/server`, drop `Bind: []interface{}{app}`, and remove the dependency
on Wails IPC entirely.

**Rejected (for v1)**: large blast radius — every binding, every desktop
test, every frontend caller would change. This is a viable v2 if Wails IPC
ever proves limiting (single-fenced WebView, reflection startup cost), but
the proxy approach lets v1 keep the existing binding surface intact.

### 3. Allow-list the localhost origin via `BindingsAllowedOrigins`, keep
   the redirect

Set `BindingsAllowedOrigins: "http://127.0.0.1:*"` and have the studio's
`index.html` fetch + execute `/wails/runtime.js` + `/wails/ipc.js`
cross-origin from the AssetServer.

**Rejected**: the `wails://` scheme on Mac/Linux is custom and WebKit
disallows cross-scheme `<script>` loads from `http://`. Even on Windows
(`http://wails.localhost`), the runtime.js served by the AssetServer
includes the dynamically-generated `window.wailsbindings = '...'` prefix
with the bindings JSON; embedding the static `vendor/` JS file alone
wouldn't include that prefix, so `window.go` would still be empty.

## Consequences

### Positive

- `window.go.main.App.*` is reachable on the studio SPA in desktop mode.
  Welcome, Settings, ProjectSwitcher, MissingCLIBanner, and the
  native-menu event subscriptions all work as designed.
- The HTTP proxy is deliberately small: it only forwards `/api/*` to the
  current loopback server, forces the target Host, and rewrites Origin to
  match the loopback target. It does not own authentication state.
- `iterion studio`, the default desktop headless daemon, and the
  in-process fallback use the same `cli.RunStudio` / `DisableAuth=true`
  local trust model; desktop only changes how the SPA is hosted and how WS
  URLs are resolved.
- The reviewer's blocker is resolved without restructuring the binding
  surface or vendoring more Wails internals.

### Negative

- Adds proxy code (`asset_proxy.go` + tests).
- Desktop REST calls have TWO HTTP servers in the request path: the Wails
  AssetServer (intercepting `wails://` / `wails.localhost`) and the selected
  loopback `pkg/server` daemon/fallback. Latency overhead is one extra
  in-process hop (negligible) but every observability story has to account
  for both layers. SPA/static responses are served directly from the GUI
  embed; only `/api/*` uses the proxy hop. We mitigate by keeping the proxy
  minimal (no caching, no body-rewriting, only rewrite-time URL/Host/Origin
  tweaks).
- WebSockets are the exception to the single AssetServer origin: they dial
  the loopback server directly because Wails rejects WS upgrades. Origin
  allow-listing in `pkg/server` must continue to include the Wails origins
  for this desktop path.
- `GetSessionToken` is still part of the desktop binding contract even
  though local desktop returns an empty string. This keeps the SPA helper
  stable today, but a future hosted-auth token flow must wire both the
  producer and the server-side consumer before documenting tokenized WS
  dials as active behaviour.

### Operational

- `docs/desktop-architecture.md` updated to reflect the AssetServer
  handler's GUI-embedded SPA/static path, `/api/*` proxy, default daemon
  mode, and WS carve-out.
- `docs/desktop-qa.md` first checkbox now exercises the actual production
  path (bindings reachable on studio), so a regression of the original
  blocker would surface immediately on the next QA pass.
