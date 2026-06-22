package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeIdP serves a minimal OIDC discovery + token + userinfo surface. issuer
// overrides the advertised issuer (empty → use the server's own URL).
func fakeIdP(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		iss := issuer
		if iss == "" {
			iss = base
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 iss,
			"authorization_endpoint": base + "/authorize",
			"token_endpoint":         base + "/token",
			"userinfo_endpoint":      base + "/userinfo",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "user-123", "email": "alice@acme.example", "email_verified": true, "name": "Alice",
		})
	})
	ts := httptest.NewServer(mux)
	base = ts.URL
	t.Cleanup(ts.Close)
	return ts
}

func TestGenericConnector_HappyPath(t *testing.T) {
	ts := fakeIdP(t, "")
	// Plain client (nil) so the loopback httptest server is reachable; strict
	// off so the http endpoints aren't rejected.
	c := NewGenericConnectorWithSlug("oidc-org-abc", ts.URL, "client", "secret", "Acme", nil, nil, false)
	if c.Name() != "oidc-org-abc" {
		t.Errorf("Name()=%q want oidc-org-abc", c.Name())
	}
	au, err := c.AuthorizeURL(context.Background(), "https://app/cb", "st4te", "verifier")
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	if !strings.Contains(au, "code_challenge_method=S256") || !strings.Contains(au, "state=st4te") {
		t.Errorf("authorize url missing pkce/state: %s", au)
	}
	ext, err := c.ExchangeCode(context.Background(), "code", "https://app/cb", "verifier")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if ext.Provider != "oidc-org-abc" || ext.Subject != "user-123" || ext.Email != "alice@acme.example" {
		t.Errorf("unexpected ext user: %+v", ext)
	}
}

func TestGenericConnector_IssuerMismatch(t *testing.T) {
	ts := fakeIdP(t, "https://evil.example/realms/x")
	c := NewGenericConnectorWithSlug("oidc-org-x", ts.URL, "client", "secret", "Acme", nil, nil, false)
	if _, err := c.AuthorizeURL(context.Background(), "https://app/cb", "s", "v"); err == nil {
		t.Fatalf("expected issuer-mismatch error")
	} else if !strings.Contains(err.Error(), "issuer mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenericConnector_StrictRequiresHTTPSEndpoints(t *testing.T) {
	ts := fakeIdP(t, "")
	// strict=true but a plain client so discovery dial to loopback succeeds and
	// fails on the http endpoint check (not the dial).
	c := NewGenericConnectorWithSlug("oidc-org-x", ts.URL, "client", "secret", "Acme", nil, nil, true)
	if _, err := c.AuthorizeURL(context.Background(), "https://app/cb", "s", "v"); err == nil {
		t.Fatalf("expected https-endpoint error in strict mode")
	} else if !strings.Contains(err.Error(), "https") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenericConnector_BackCompatSlug(t *testing.T) {
	c := NewGenericConnector("https://sso.example", "id", "secret", "", nil)
	if c.Name() != "sso" {
		t.Errorf("back-compat slug=%q want sso", c.Name())
	}
	if c.Display() != "SSO" {
		t.Errorf("default display=%q want SSO", c.Display())
	}
}
