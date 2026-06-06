---
name: reproducer-go
description: |
  How to write a focused Go regression test per finding_type — one
  that FAILS on unpatched code and PASSES after the fix. Loaded by
  `[[patch]]`'s author phase when language=go and the project has
  a `_test.go` suite.
---

# reproducer-go — failing-then-passing tests per finding_type

Each recipe writes ONE test that:

- Fails on the **unpatched** code (the test is the oracle —
  without it, the fix is unprovable).
- Passes after the fix lands.
- Lives in a `_test.go` file next to the unit it exercises.
- Runs in isolation: `go test -run <Name>` reproduces it in <1s.

Use Go idioms: `t.Run` for subcases, table tests for variants,
`net/http/httptest` for handlers, `t.Helper()` on shared helpers.

## General shape

```go
func TestReproduce<FindingType>_<short>(t *testing.T) {
    // arrange: minimal fixture
    // act:     hit the sink with the attacker input
    // assert:  the bad behavior must NOT happen
}
```

Avoid sleeps, network calls, or filesystem mounts outside `t.TempDir()`.

## Recipes per finding_type

### injection (SQL / command / template)

Table-driven, one row per injection vector. Failing assertion: the
crafted input must not flow into the underlying string.

```go
func TestReproduce_SQLInjection_OrderLookup(t *testing.T) {
    db, mock := newDBMock(t)
    cases := []struct {
        name, in, wantQuery string
    }{
        {"benign",  "42",            "SELECT * FROM orders WHERE id = $1"},
        {"attack", "1 OR 1=1 --",   "SELECT * FROM orders WHERE id = $1"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mock.ExpectQuery(regexp.QuoteMeta(tc.wantQuery)).WithArgs(tc.in)
            _, err := LookupOrder(db, tc.in)
            if err != nil { t.Fatalf("LookupOrder: %v", err) }
            if err := mock.ExpectationsWereMet(); err != nil {
                t.Fatalf("query was not parameterized: %v", err)
            }
        })
    }
}
```

For `os/exec` injection, assert the exec'd argv slice instead of a
shell string: `cmd.Args` must contain the user input as a single
element, not spliced into a `/bin/sh -c` string.

### xss

Use `httptest.NewServer` + a hand-rolled handler. Assertion: the
response body must HTML-escape the attacker payload.

```go
func TestReproduce_XSS_UserProfile(t *testing.T) {
    h := NewProfileHandler(stubStore{name: `<script>alert(1)</script>`})
    srv := httptest.NewServer(h); defer srv.Close()
    resp, _ := http.Get(srv.URL + "/u/42")
    body, _ := io.ReadAll(resp.Body)
    if bytes.Contains(body, []byte("<script>")) {
        t.Fatal("user-controlled name reflected unescaped")
    }
}
```

### ssrf

Use `httptest` for the "external" side and assert the outbound URL
is denied by the allowlist BEFORE the call leaves the process.

```go
func TestReproduce_SSRF_ProxyDeniesInternalHost(t *testing.T) {
    var hit bool
    backend := httptest.NewServer(http.HandlerFunc(
        func(w http.ResponseWriter, _ *http.Request) { hit = true }))
    defer backend.Close()

    // Attack: backend.URL points at 127.0.0.1 — must be rejected.
    _, err := ProxyFetch(context.Background(), backend.URL)
    if err == nil {
        t.Fatalf("expected ssrf guard to reject %s", backend.URL)
    }
    if hit {
        t.Fatal("outbound request reached internal host")
    }
}
```

### auth

`httptest` + missing/forged token. Assertion: handler returns 401
for missing creds and 403 for non-matching scope.

```go
func TestReproduce_Auth_RequiresBearer(t *testing.T) {
    h := NewAdminRouter(authStub{})
    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/admin/users", nil)
    h.ServeHTTP(rec, req)
    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("missing token allowed: got %d", rec.Code)
    }
}
```

### authz / idor

Two tenants, same endpoint. Assertion: tenant A's token MUST NOT
read tenant B's resource.

```go
func TestReproduce_IDOR_OrdersTenantIsolation(t *testing.T) {
    db := seedTwoTenants(t)
    h := NewOrdersHandler(db)
    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/orders/B-1", nil)
    req.Header.Set("Authorization", "Bearer "+tokenForTenant("A"))
    h.ServeHTTP(rec, req)
    if rec.Code == http.StatusOK {
        t.Fatalf("tenant A read tenant B order: %s", rec.Body.String())
    }
}
```

### crypto — note → hard-stop

Crypto findings do NOT get auto-patched. See `[[crypto-handling]]`.
This skill still describes the test shape for humans who write
the fix:

- Constant-time comparison: use `crypto/subtle.ConstantTimeCompare`
  and assert the variable-time helper is gone via `go/ast` parse —
  not via timing, which is flaky in CI.
- Weak RNG: assert `rand.Reader` is the source (`crypto/rand`),
  not `math/rand`.
- Weak cipher: assert the negotiated suite is in an allowlist.

Test shape is informational; do NOT land an auto-generated crypto
fix.

### secrets

Static-only: a `go test` that scans a build artifact (or a
committed config file) for high-entropy strings. Typically the
real assertion is `gitleaks` exit-code 0 at CI time, not a unit
test. If you must write one:

```go
func TestReproduce_NoEmbeddedAWSKey(t *testing.T) {
    bin, _ := os.ReadFile("./bin/server")
    if bytes.Contains(bin, []byte("AKIA")) {
        t.Fatal("AWS key prefix present in build artifact")
    }
}
```

### deserialization

For `encoding/gob`, `gopkg.in/yaml.v3`, `encoding/json` with
arbitrary types, assert that the deserializer is given a typed
target (struct) and rejects an attacker payload that expects a
wider type.

```go
func TestReproduce_Deserial_RejectsTypeConfusion(t *testing.T) {
    var out OrderRequest
    payload := []byte(`{"id":"x","__proto__":{"admin":true}}`)
    err := json.Unmarshal(payload, &out)
    if err != nil { /* unmarshal accepting unknown fields is fine */ }
    if out.Admin { t.Fatal("admin flag flowed via unknown field") }
}
```

### path-trav

Use `t.TempDir()` and attempt `../../etc/passwd`. Assertion: the
helper MUST reject paths that escape the base.

```go
func TestReproduce_PathTrav_Download(t *testing.T) {
    base := t.TempDir()
    _, err := SafeOpen(base, "../../etc/passwd")
    if err == nil { t.Fatal("path escape allowed") }
}
```

### redirect

`httptest` recorder; assert the `Location` header is on the
allowlist.

```go
func TestReproduce_OpenRedirect(t *testing.T) {
    h := NewLoginHandler()
    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/login?next=https://evil.example/", nil)
    h.ServeHTTP(rec, req)
    loc := rec.Header().Get("Location")
    if strings.HasPrefix(loc, "https://evil.example") {
        t.Fatalf("redirected off-domain: %s", loc)
    }
}
```

### config

A `go test` that parses the runtime config and asserts secure
defaults:

```go
func TestReproduce_TLS_MinVersion(t *testing.T) {
    cfg := LoadServerTLSConfig()
    if cfg.MinVersion < tls.VersionTLS12 {
        t.Fatalf("MinVersion=%v allows pre-TLS 1.2", cfg.MinVersion)
    }
}
```

### other

Pick the closest pattern above. The bar is still
"fails-on-unpatched, passes-after".

## Reproducer hygiene

- One test per finding. Don't bundle.
- Name as `TestReproduce_<FindingType>_<ShortHint>` so
  `go test -run` can target it cleanly.
- Use `t.Parallel()` only when the test has no shared mutable
  state.
- Don't import the production binary's `main`; test the unit
  (`Handler`, `Validator`, ...) directly.

## See also

- `[[patch]]` — invokes this skill in step 6 of the author brief.
- `[[reproducer-ts]]` — same recipes for TS/JS.
- `[[crypto-handling]]` — why crypto findings don't get
  auto-patched.
- `[[finding-taxonomy]]` — the twelve categories.
