package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/auth/orgsso"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/store"
)

// registerOrgSSORoutes wires the per-tenant SSO provider CRUD. An org admin
// self-serves their org's Keycloak (and, from Phase 2, a GitHub team
// allow-list), mirroring the forge OAuth-app routes: tenant-scoped, sealed
// secrets, admin-gated, audited.
func (s *Server) registerOrgSSORoutes() {
	s.mux.Handle("GET /api/teams/{id}/sso/providers", s.requireAuth(http.HandlerFunc(s.handleListOrgSSOProviders)))
	s.mux.Handle("POST /api/teams/{id}/sso/providers", s.requireAuth(http.HandlerFunc(s.handleCreateOrgSSOProvider)))
	s.mux.Handle("PATCH /api/teams/{id}/sso/providers/{provider_id}", s.requireAuth(http.HandlerFunc(s.handleUpdateOrgSSOProvider)))
	s.mux.Handle("DELETE /api/teams/{id}/sso/providers/{provider_id}", s.requireAuth(http.HandlerFunc(s.handleDeleteOrgSSOProvider)))
	s.mux.Handle("POST /api/teams/{id}/sso/providers/{provider_id}/test", s.requireAuth(http.HandlerFunc(s.handleTestOrgSSOProvider)))
}

// orgSSOProviderReq is the create/update body. client_secret is write-only; an
// empty value on PATCH means "don't rotate the stored secret".
type orgSSOProviderReq struct {
	Kind         string   `json:"kind"`
	DisplayName  string   `json:"display_name"`
	Enabled      bool     `json:"enabled"`
	IssuerURL    string   `json:"issuer_url"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	DefaultRole  string   `json:"default_role"`
	// GitHub fields (wired in Phase 2).
	AutoProvision bool                     `json:"auto_provision"`
	Grants        []orgsso.GitHubTeamGrant `json:"grants"`
}

// orgSSOProviderView is the response shape: the stored row (SealedSecret is
// json:"-") plus the redirect URI an admin registers at their IdP.
type orgSSOProviderView struct {
	orgsso.OrgSSOProvider
	RedirectURI string `json:"redirect_uri,omitempty"`
}

func (s *Server) viewOrgSSOProvider(p orgsso.OrgSSOProvider) orgSSOProviderView {
	p.SealedSecret = nil // defensive — also json:"-"
	v := orgSSOProviderView{OrgSSOProvider: p}
	if p.Kind == orgsso.KindOIDC {
		v.RedirectURI = s.oidcRedirectURI(p.OIDCSlug())
	}
	return v
}

func (s *Server) handleListOrgSSOProviders(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	rows, err := s.orgSSO.ListByTenant(store.WithTenant(r.Context(), teamID), teamID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "list sso providers: %v", err)
		return
	}
	views := make([]orgSSOProviderView, 0, len(rows))
	for _, p := range rows {
		views = append(views, s.viewOrgSSOProvider(p))
	}
	writeJSON(w, map[string]any{"providers": views})
}

func (s *Server) handleCreateOrgSSOProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	var req orgSSOProviderReq
	if !decodeJSON(w, r, &req) {
		return
	}
	now := time.Now().UTC()
	row := orgsso.OrgSSOProvider{
		ID:        uuid.NewString(),
		TenantID:  teamID,
		Kind:      orgsso.Kind(strings.TrimSpace(req.Kind)),
		Enabled:   req.Enabled,
		CreatedBy: id.UserID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.applyOrgSSOReq(&row, req, true); err != nil {
		httpError(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := s.orgSSO.Create(store.WithTenant(r.Context(), teamID), row); err != nil {
		s.writeOrgSSOError(w, err)
		return
	}
	s.auditTenant(r, teamID, "org_sso.created", "org_sso_provider", row.ID, map[string]any{
		"kind": string(row.Kind), "enabled": row.Enabled,
	})
	writeJSON(w, s.viewOrgSSOProvider(row))
}

func (s *Server) handleUpdateOrgSSOProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	providerID := r.PathValue("provider_id")
	ctx := store.WithTenant(r.Context(), teamID)
	row, err := s.orgSSO.Get(ctx, providerID)
	if err != nil || row.TenantID != teamID {
		httpError(w, http.StatusNotFound, "provider not found")
		return
	}
	var req orgSSOProviderReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// Kind is immutable; ignore any kind in the body.
	req.Kind = string(row.Kind)
	row.Enabled = req.Enabled
	row.UpdatedAt = time.Now().UTC()
	if err := s.applyOrgSSOReq(&row, req, false); err != nil {
		httpError(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := s.orgSSO.Update(ctx, row); err != nil {
		s.writeOrgSSOError(w, err)
		return
	}
	s.auditTenant(r, teamID, "org_sso.updated", "org_sso_provider", row.ID, map[string]any{
		"kind": string(row.Kind), "enabled": row.Enabled,
	})
	writeJSON(w, s.viewOrgSSOProvider(row))
}

func (s *Server) handleDeleteOrgSSOProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	providerID := r.PathValue("provider_id")
	ctx := store.WithTenant(r.Context(), teamID)
	row, err := s.orgSSO.Get(ctx, providerID)
	if err != nil || row.TenantID != teamID {
		httpError(w, http.StatusNotFound, "provider not found")
		return
	}
	if err := s.orgSSO.Delete(ctx, providerID); err != nil {
		httpError(w, http.StatusInternalServerError, "delete provider: %v", err)
		return
	}
	s.auditTenant(r, teamID, "org_sso.deleted", "org_sso_provider", providerID, map[string]any{"kind": string(row.Kind)})
	w.WriteHeader(http.StatusNoContent)
}

// handleTestOrgSSOProvider runs a discovery smoke test against an OIDC row's
// issuer (through the SSRF guard) so an admin can validate config before
// enabling it. Never returns the client secret or the upstream body.
func (s *Server) handleTestOrgSSOProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	providerID := r.PathValue("provider_id")
	row, err := s.orgSSO.Get(store.WithTenant(r.Context(), teamID), providerID)
	if err != nil || row.TenantID != teamID {
		httpError(w, http.StatusNotFound, "provider not found")
		return
	}
	if row.Kind != orgsso.KindOIDC {
		httpError(w, http.StatusBadRequest, "test is only supported for oidc providers")
		return
	}
	secret, err := orgsso.OpenClientSecret(s.sealer, row.ID, row.SealedSecret)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "open secret")
		return
	}
	strict := s.ssoStrict()
	conn := oidc.NewGenericConnectorWithSlug(row.OIDCSlug(), row.IssuerURL, row.ClientID, secret, row.DisplayName, row.Scopes, oidc.SafeGenericClient(strict), strict)
	// AuthorizeURL triggers discovery; a build success means the issuer is
	// reachable, advertises matching endpoints, and (strict) is https.
	if _, derr := conn.AuthorizeURL(r.Context(), s.oidcRedirectURI(row.OIDCSlug()), "test-state", "test-verifier"); derr != nil {
		writeJSON(w, map[string]any{"ok": false, "error": derr.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// applyOrgSSOReq validates a request against the row's kind and applies the
// mutable fields. On create, the secret is required for OIDC; on update an
// empty client_secret preserves the stored one. The role ceiling is enforced
// by orgsso.Validate (which rejects owner) — anyone reaching here passed
// canManageTeam (admin/owner), whose SSO grant ceiling is exactly "admin".
func (s *Server) applyOrgSSOReq(row *orgsso.OrgSSOProvider, req orgSSOProviderReq, create bool) error {
	row.DisplayName = req.DisplayName
	switch orgsso.Kind(req.Kind) {
	case orgsso.KindOIDC:
		row.Kind = orgsso.KindOIDC
		row.IssuerURL = req.IssuerURL
		row.ClientID = req.ClientID
		row.Scopes = req.Scopes
		row.DefaultRole = identity.Role(strings.TrimSpace(req.DefaultRole))
		if req.ClientSecret != "" {
			sealed, err := orgsso.SealClientSecret(s.sealer, row.ID, req.ClientSecret)
			if err != nil {
				return err
			}
			row.SealedSecret = sealed
		} else if create {
			return orgsso.ErrInvalid // client_secret required on create
		}
		// (update with empty secret keeps row.SealedSecret unchanged)
	case orgsso.KindGitHub:
		// Phase 2 wires the GitHub team→org grant logic + proof-of-control.
		// Until then the CRUD refuses github rows so an admin can't create a
		// silently-inert allow-list.
		return errOrgSSOGitHubNotYet
	default:
		return orgsso.ErrInvalid
	}
	row.Normalize()
	return row.Validate()
}

var errOrgSSOGitHubNotYet = orgSSOClientError("github SSO team-gating is not enabled yet")

// orgSSOClientError is a small error type for 400-class messages.
type orgSSOClientError string

func (e orgSSOClientError) Error() string { return string(e) }

func (s *Server) writeOrgSSOError(w http.ResponseWriter, err error) {
	switch {
	case err == orgsso.ErrExists:
		httpError(w, http.StatusConflict, "a github provider already exists for this org")
	case err == orgsso.ErrInvalid || err == orgsso.ErrOwnerNotGrant:
		httpError(w, http.StatusBadRequest, "%v", err)
	default:
		httpError(w, http.StatusInternalServerError, "%v", err)
	}
}
