package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/auth/orgsso"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/secrets"
)

func newOrgSSOTestServer(t *testing.T) *Server {
	t.Helper()
	s := newOrgTestServer(t)
	s.orgSSO = orgsso.NewMemoryStore()
	s.cfg.PublicURL = "https://iterion.test"
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	sealer, err := secrets.NewAESGCMSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	s.sealer = sealer
	if _, err := s.authStore().CreateTeam(context.Background(), identity.Team{ID: "t1", Name: "Acme", Slug: "acme"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func ssoReq(ctx context.Context, method, path, body, teamID, providerID string) *http.Request {
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
	if providerID != "" {
		r.SetPathValue("provider_id", providerID)
	}
	return r
}

func TestOrgSSO_OIDC_CRUD(t *testing.T) {
	s := newOrgSSOTestServer(t)
	ctx := superAdminCtx()
	const base = "/api/teams/t1/sso/providers"

	// Create OIDC provider.
	w := httptest.NewRecorder()
	body := `{"kind":"oidc","display_name":"Acme KC","enabled":true,"issuer_url":"https://sso.acme.example/realms/main","client_id":"iterion","client_secret":"s3cr3t","default_role":"member"}`
	s.handleCreateOrgSSOProvider(w, ssoReq(ctx, "POST", base, body, "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}
	// Secret must never be serialised; redirect_uri must be present.
	if strings.Contains(w.Body.String(), "s3cr3t") || strings.Contains(w.Body.String(), "sealed") {
		t.Fatalf("secret leaked in response: %s", w.Body.String())
	}
	var created orgSSOProviderView
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.RedirectURI == "" {
		t.Fatalf("created=%+v", created)
	}
	if !strings.Contains(created.RedirectURI, "oidc-org-"+created.ID) {
		t.Errorf("redirect_uri=%q should embed the per-org slug", created.RedirectURI)
	}

	// List.
	w = httptest.NewRecorder()
	s.handleListOrgSSOProviders(w, ssoReq(ctx, "GET", base, "", "t1", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), created.ID) {
		t.Fatalf("list: code=%d body=%s", w.Code, w.Body.String())
	}

	// Update (rename; empty client_secret keeps the stored one).
	w = httptest.NewRecorder()
	upd := `{"kind":"oidc","display_name":"Renamed","enabled":true,"issuer_url":"https://sso.acme.example/realms/main","client_id":"iterion","default_role":"member"}`
	s.handleUpdateOrgSSOProvider(w, ssoReq(ctx, "PATCH", base+"/"+created.ID, upd, "t1", created.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("update: code=%d body=%s", w.Code, w.Body.String())
	}
	row, _ := s.orgSSO.Get(context.Background(), created.ID)
	if row.DisplayName != "Renamed" || len(row.SealedSecret) == 0 {
		t.Fatalf("update lost rename or wiped secret: %+v sealedLen=%d", row.DisplayName, len(row.SealedSecret))
	}

	// Delete.
	w = httptest.NewRecorder()
	s.handleDeleteOrgSSOProvider(w, ssoReq(ctx, "DELETE", base+"/"+created.ID, "", "t1", created.ID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: code=%d", w.Code)
	}
}

func TestOrgSSO_GitHubCreate(t *testing.T) {
	s := newOrgSSOTestServer(t)
	ctx := superAdminCtx()
	const base = "/api/teams/t1/sso/providers"

	// A member-grant github row is accepted.
	w := httptest.NewRecorder()
	body := `{"kind":"github","enabled":true,"auto_provision":true,"grants":[{"github_org":"acme","team_slug":"eng","role":"member"}]}`
	s.handleCreateOrgSSOProvider(w, ssoReq(ctx, "POST", base, body, "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("github create: code=%d body=%s", w.Code, w.Body.String())
	}

	// A second github row for the same org is refused (one allow-list per org).
	w = httptest.NewRecorder()
	s.handleCreateOrgSSOProvider(w, ssoReq(ctx, "POST", base, body, "t1", ""))
	if w.Code != http.StatusConflict {
		t.Fatalf("second github row: code=%d want 409", w.Code)
	}

	// An admin-role grant is rejected (github grants are capped at member).
	w = httptest.NewRecorder()
	adminGrant := `{"kind":"github","enabled":true,"grants":[{"github_org":"beta","team_slug":"x","role":"admin"}]}`
	s.handleCreateOrgSSOProvider(w, ssoReq(ctx, "POST", "/api/teams/t2/sso/providers", adminGrant, "t2", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("admin github grant: code=%d want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestOrgSSO_NonAdminForbidden(t *testing.T) {
	s := newOrgSSOTestServer(t)
	ctx := context.Background()
	if err := s.authStore().UpsertMembership(ctx, identity.Membership{UserID: "m", TeamID: "t1", Role: identity.RoleMember}); err != nil {
		t.Fatal(err)
	}
	member := auth.WithIdentity(ctx, auth.Identity{UserID: "m", TeamID: "t1", Role: identity.RoleMember})
	body := `{"kind":"oidc","enabled":true,"issuer_url":"https://sso.x/realms/m","client_id":"c","client_secret":"s"}`
	w := httptest.NewRecorder()
	s.handleCreateOrgSSOProvider(w, ssoReq(member, "POST", "/api/teams/t1/sso/providers", body, "t1", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("member create: code=%d want 403", w.Code)
	}
	// A member CAN view.
	w = httptest.NewRecorder()
	s.handleListOrgSSOProviders(w, ssoReq(member, "GET", "/api/teams/t1/sso/providers", "", "t1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("member list: code=%d want 200", w.Code)
	}
}

func TestOrgSSO_CrossTenantNotFound(t *testing.T) {
	s := newOrgSSOTestServer(t)
	ctx := superAdminCtx()
	// Create in t1.
	w := httptest.NewRecorder()
	body := `{"kind":"oidc","enabled":true,"issuer_url":"https://sso.acme/realms/m","client_id":"c","client_secret":"s"}`
	s.handleCreateOrgSSOProvider(w, ssoReq(ctx, "POST", "/api/teams/t1/sso/providers", body, "t1", ""))
	var created orgSSOProviderView
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	// Delete via a different tenant path → 404 (row belongs to t1).
	w = httptest.NewRecorder()
	s.handleDeleteOrgSSOProvider(w, ssoReq(ctx, "DELETE", "/api/teams/t2/sso/providers/"+created.ID, "", "t2", created.ID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete: code=%d want 404", w.Code)
	}
}

func TestListProviders_ByOrgSlug(t *testing.T) {
	s := newOrgSSOTestServer(t)
	// Seed an enabled OIDC row for team acme.
	if err := s.orgSSO.Create(context.Background(), orgsso.OrgSSOProvider{
		ID: "p1", TenantID: "t1", Kind: orgsso.KindOIDC, Enabled: true,
		DisplayName: "Acme KC", IssuerURL: "https://sso.acme/realms/m", ClientID: "c",
	}); err != nil {
		t.Fatal(err)
	}
	// With ?org=acme the per-org provider appears.
	w := httptest.NewRecorder()
	s.handleListProviders(w, httptest.NewRequest("GET", "/api/auth/providers?org=acme", nil))
	if !strings.Contains(w.Body.String(), "oidc-org-p1") {
		t.Fatalf("expected per-org provider for slug acme: %s", w.Body.String())
	}
	// Unknown slug → no per-org providers, and NOT a 404 (no org-existence oracle).
	w = httptest.NewRecorder()
	s.handleListProviders(w, httptest.NewRequest("GET", "/api/auth/providers?org=ghost", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "oidc-org-") {
		t.Fatalf("unknown slug should leak nothing: code=%d body=%s", w.Code, w.Body.String())
	}
}
