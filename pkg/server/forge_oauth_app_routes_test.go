package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/forge"
	"github.com/SocialGouv/iterion/pkg/secrets"
)

func newForgeOAuthAppTestServer(t *testing.T) *Server {
	t.Helper()
	s := newOrgTestServer(t)
	s.forgeOAuthApps = forge.NewMemoryOAuthAppStore()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	sealer, err := secrets.NewAESGCMSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	s.sealer = sealer
	return s
}

func oauthAppReq(ctx context.Context, method, path, body, teamID, appID string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	r = r.WithContext(ctx)
	if teamID != "" {
		r.SetPathValue("id", teamID)
	}
	if appID != "" {
		r.SetPathValue("app_id", appID)
	}
	return r
}

func TestForgeOAuthApp_CRUD(t *testing.T) {
	s := newForgeOAuthAppTestServer(t)
	ctx := superAdminCtx()
	const base = "/api/teams/t1/forge/oauth-apps"

	// missing client_secret → 400
	w := httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, `{"provider":"gitlab","mode":"manual","client_id":"cid"}`, "t1", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing secret: code=%d body=%s", w.Code, w.Body.String())
	}

	// unknown provider → 400
	w = httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, `{"provider":"bogus","mode":"manual","client_id":"c","client_secret":"x"}`, "t1", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad provider: code=%d", w.Code)
	}

	// auto mode is not available yet in this phase → 400
	w = httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, `{"provider":"gitlab","mode":"auto","admin_token":"t"}`, "t1", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("auto mode (phase 1): code=%d", w.Code)
	}

	// valid manual create → 200, secret not serialised
	w = httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, `{"provider":"gitlab","forge_base_url":"https://gitlab.example.com","mode":"manual","client_id":"cid","client_secret":"s3cr3t"}`, "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}
	var created forge.ForgeOAuthApp
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.ClientID != "cid" {
		t.Fatalf("created = %+v", created)
	}
	if len(created.SealedSecret) != 0 {
		t.Fatalf("sealed secret leaked in response")
	}
	if created.ForgeBaseURL != "https://gitlab.example.com" {
		t.Fatalf("base url not canonicalised: %q", created.ForgeBaseURL)
	}

	// the connect resolver finds it (store-first OAuth client build), with a
	// non-canonical base URL to prove canonicalisation matches.
	if _, ok := s.forgeOAuthAppFor(ctx, "t1", forge.ProviderGitLab, "gitlab.example.com/"); !ok {
		t.Fatal("forgeOAuthAppFor did not resolve the registered app")
	}

	// duplicate instance → 409
	w = httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, `{"provider":"gitlab","forge_base_url":"https://gitlab.example.com","mode":"manual","client_id":"other","client_secret":"y"}`, "t1", ""))
	if w.Code != http.StatusConflict {
		t.Fatalf("dup: code=%d body=%s", w.Code, w.Body.String())
	}

	// list → 1
	w = httptest.NewRecorder()
	s.handleListForgeOAuthApps(w, oauthAppReq(ctx, "GET", base, "", "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: code=%d", w.Code)
	}
	var listResp struct {
		Apps []forge.ForgeOAuthApp `json:"apps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Apps) != 1 {
		t.Fatalf("list len=%d", len(listResp.Apps))
	}

	// cross-tenant delete (app is t1's, requested as t2) → 404
	w = httptest.NewRecorder()
	s.handleDeleteForgeOAuthApp(w, oauthAppReq(ctx, "DELETE", base+"/"+created.ID, "", "t2", created.ID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete: code=%d", w.Code)
	}

	// delete → 204
	w = httptest.NewRecorder()
	s.handleDeleteForgeOAuthApp(w, oauthAppReq(ctx, "DELETE", base+"/"+created.ID, "", "t1", created.ID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestForgeOAuthApp_AutoCreate(t *testing.T) {
	gl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/applications" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "application_id": "auto-cid", "secret": "auto-sec"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gl.Close()

	s := newForgeOAuthAppTestServer(t)
	ctx := superAdminCtx()
	const base = "/api/teams/t1/forge/oauth-apps"

	body := `{"provider":"gitlab","forge_base_url":"` + gl.URL + `","mode":"auto","admin_token":"admintok"}`
	w := httptest.NewRecorder()
	s.handleRegisterForgeOAuthApp(w, oauthAppReq(ctx, "POST", base, body, "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("auto create: code=%d body=%s", w.Code, w.Body.String())
	}
	var created forge.ForgeOAuthApp
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ClientID != "auto-cid" || !created.AutoCreated || created.ProviderAppID != "9" {
		t.Fatalf("created = %+v", created)
	}
	// the resolver opens the sealed secret + builds the OAuth client for it
	if _, ok := s.forgeOAuthAppFor(ctx, "t1", forge.ProviderGitLab, gl.URL); !ok {
		t.Fatal("resolver did not find the auto-created app")
	}
}
