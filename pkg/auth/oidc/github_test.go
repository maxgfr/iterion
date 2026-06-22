package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// rewriteTransport redirects every request to base (the httptest server),
// preserving path+query — so the connector's hardcoded github.com /
// api.github.com URLs hit the fake server.
type rewriteTransport struct{ host, scheme string }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.scheme
	req.URL.Host = rt.host
	return http.DefaultTransport.RoundTrip(req)
}

func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at", "token_type": "bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "alice", "name": "Alice"})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"email": "alice@acme.example", "primary": true, "verified": true}})
	})
	mux.HandleFunc("/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"login": "Acme"}, {"login": "beta"}})
	})
	mux.HandleFunc("/user/teams", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"slug": "Eng", "organization": map[string]any{"login": "Acme"}},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestGitHubConnector_FetchesOrgsAndTeams(t *testing.T) {
	ts := fakeGitHub(t)
	u, _ := url.Parse(ts.URL)
	c := NewGitHubConnector("cid", "secret", "GitHub")
	c.httpClient.Transport = rewriteTransport{host: u.Host, scheme: u.Scheme}

	ext, err := c.ExchangeCode(context.Background(), "code", "https://app/cb", "verifier")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if ext.Provider != "github" || ext.Subject != "42" || ext.Email != "alice@acme.example" {
		t.Errorf("unexpected ext: %+v", ext)
	}
	got := append([]string(nil), ext.Groups...)
	sort.Strings(got)
	want := []string{"acme/*", "acme/eng", "beta/*"} // lowercased org/* + org/team
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("groups = %v, want %v", got, want)
	}
}

func TestGitHubConnector_ScopeIncludesReadOrg(t *testing.T) {
	c := NewGitHubConnector("cid", "secret", "")
	au, err := c.AuthorizeURL(context.Background(), "https://app/cb", "state", "verifier")
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	if !strings.Contains(au, "read%3Aorg") && !strings.Contains(au, "read:org") {
		t.Errorf("authorize URL missing read:org scope: %s", au)
	}
}

func TestNextGitHubLink(t *testing.T) {
	h := `<https://api.github.com/user/orgs?page=2>; rel="next", <https://api.github.com/user/orgs?page=5>; rel="last"`
	if got := nextGitHubLink(h); got != "https://api.github.com/user/orgs?page=2" {
		t.Errorf("next = %q", got)
	}
	if got := nextGitHubLink(`<https://x>; rel="last"`); got != "" {
		t.Errorf("expected no next, got %q", got)
	}
}
