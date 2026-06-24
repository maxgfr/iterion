package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/auth"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/marketplace"
)

func newCloudMarketplaceServer(t *testing.T) *Server {
	t.Helper()
	store, err := marketplace.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Config{
		DisableAuth: true, // skip the auth middleware; we inject identities directly
		Mode:        "cloud",
		Marketplace: store,
	}, iterlog.New(iterlog.LevelError, nil))
	srv.handler = srv.mux
	return srv
}

// doAs issues a request carrying the given identity in its context (the
// auth middleware is bypassed in tests, so handlers read it via
// auth.FromContext).
func doAs(t *testing.T, srv *Server, id *auth.Identity, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if id != nil {
		r = r.WithContext(auth.WithIdentity(r.Context(), *id))
	}
	rec := httptest.NewRecorder()
	srv.handler.ServeHTTP(rec, r)
	return rec
}

func TestMarketplace_CloudModerationFlow(t *testing.T) {
	repo := t.TempDir()
	writeFixtureBundle(t, repo, "cloudbot")
	srv := newCloudMarketplaceServer(t)

	user := &auth.Identity{UserID: "u1", TeamID: "orgA"}
	admin := &auth.Identity{UserID: "root", IsSuperAdmin: true}

	// Config advertises the cloud scope set.
	rec := doAs(t, srv, nil, http.MethodGet, "/api/v1/marketplace/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; %s", rec.Code, rec.Body.String())
	}
	var cfg marketplaceConfigResponse
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Mode != "cloud" || !cfg.Moderated || len(cfg.Scopes) == 0 {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	// Anonymous submit is rejected (auth required in cloud).
	if rec := doAs(t, srv, nil, http.MethodPost, "/api/v1/marketplace/submit", `{"repo_url":"`+repo+`","scope":"public"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon submit = %d, want 401", rec.Code)
	}

	// A user submits a public-scope bot → lands pending.
	rec = doAs(t, srv, user, http.MethodPost, "/api/v1/marketplace/submit", `{"repo_url":"`+repo+`","scope":"public"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit = %d; %s", rec.Code, rec.Body.String())
	}
	var entry marketplace.Entry
	json.Unmarshal(rec.Body.Bytes(), &entry)
	if entry.Status != marketplace.StatusPending || entry.SubmittedBy != "u1" {
		t.Fatalf("submitted entry not pending/owned: %+v", entry)
	}

	// Anonymous browse does not see the pending entry.
	rec = doAs(t, srv, nil, http.MethodGet, "/api/v1/marketplace/bots", "")
	var list struct {
		Bots []marketplace.Entry `json:"bots"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Bots) != 0 {
		t.Fatalf("anon browse showed pending entry: %+v", list.Bots)
	}

	// The submitter sees their own pending entry.
	rec = doAs(t, srv, user, http.MethodGet, "/api/v1/marketplace/bots", "")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Bots) != 1 {
		t.Fatalf("owner browse = %d entries, want 1", len(list.Bots))
	}

	// Moderation queue: a regular user is forbidden; super-admin sees it.
	if rec := doAs(t, srv, user, http.MethodGet, "/api/v1/marketplace/moderation", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("user moderation list = %d, want 403", rec.Code)
	}
	rec = doAs(t, srv, admin, http.MethodGet, "/api/v1/marketplace/moderation", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin moderation list = %d; %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Bots) != 1 || list.Bots[0].Slug != "cloudbot" {
		t.Fatalf("moderation queue = %+v", list.Bots)
	}

	// A non-admin cannot approve; the super-admin can.
	if rec := doAs(t, srv, user, http.MethodPost, "/api/v1/marketplace/moderation/cloudbot/approve", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("user approve = %d, want 403", rec.Code)
	}
	if rec := doAs(t, srv, admin, http.MethodPost, "/api/v1/marketplace/moderation/cloudbot/approve", ""); rec.Code != http.StatusOK {
		t.Fatalf("admin approve = %d; %s", rec.Code, rec.Body.String())
	}

	// Now anonymous browse sees the approved public entry.
	rec = doAs(t, srv, nil, http.MethodGet, "/api/v1/marketplace/bots", "")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Bots) != 1 || list.Bots[0].Status != marketplace.StatusApproved {
		t.Fatalf("post-approve anon browse = %+v", list.Bots)
	}
}
