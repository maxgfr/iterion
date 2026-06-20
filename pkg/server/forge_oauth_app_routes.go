package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/forge"
	"github.com/SocialGouv/iterion/pkg/store"
)

// registerForgeOAuthAppRoutes wires the per-tenant forge OAuth-app credential
// CRUD. These replace the legacy process-global env map: an admin registers (or
// later auto-creates) an OAuth app per (provider, instance), and the connect
// flow resolves it from the store — no env var, no redeploy.
func (s *Server) registerForgeOAuthAppRoutes() {
	s.mux.Handle("GET /api/teams/{id}/forge/oauth-apps", s.requireAuth(http.HandlerFunc(s.handleListForgeOAuthApps)))
	s.mux.Handle("POST /api/teams/{id}/forge/oauth-apps", s.requireAuth(http.HandlerFunc(s.handleRegisterForgeOAuthApp)))
	s.mux.Handle("DELETE /api/teams/{id}/forge/oauth-apps/{app_id}", s.requireAuth(http.HandlerFunc(s.handleDeleteForgeOAuthApp)))
}

// forgeOAuthAppReq registers an OAuth app for a (provider, instance). mode
// "manual" pastes an existing client_id/client_secret; the "auto" /
// "auto_from_connection" modes (which call the forge's create-app API) are
// added with provider auto-create support.
type forgeOAuthAppReq struct {
	Provider     string `json:"provider"`
	ForgeBaseURL string `json:"forge_base_url,omitempty"`
	Mode         string `json:"mode,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func (s *Server) handleListForgeOAuthApps(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	apps, err := s.forgeOAuthApps.ListByTenant(store.WithTenant(r.Context(), teamID), teamID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "list oauth apps: %v", err)
		return
	}
	for i := range apps {
		apps[i].SealedSecret = nil // defensive — also json:"-"
	}
	writeJSON(w, map[string]any{"apps": apps})
}

func (s *Server) handleRegisterForgeOAuthApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	var req forgeOAuthAppReq
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	provider := forge.Provider(strings.TrimSpace(req.Provider))
	if !provider.Valid() {
		httpError(w, http.StatusBadRequest, "unknown provider %q", req.Provider)
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" {
		httpError(w, http.StatusBadRequest, "unsupported mode %q — paste client_id/client_secret with mode=manual (auto-create lands with provider support)", mode)
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	clientSecret := strings.TrimSpace(req.ClientSecret)
	if clientID == "" || clientSecret == "" {
		httpError(w, http.StatusBadRequest, "client_id and client_secret are required for mode=manual")
		return
	}
	app, err := s.createForgeOAuthApp(r, teamID, id.UserID, provider, req.ForgeBaseURL, clientID, clientSecret, "", false, mode)
	if err != nil {
		s.writeForgeOAuthAppError(w, err)
		return
	}
	writeJSON(w, app)
}

func (s *Server) handleDeleteForgeOAuthApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	appID := r.PathValue("app_id")
	ctx := store.WithTenant(r.Context(), teamID)
	app, err := s.forgeOAuthApps.Get(ctx, appID)
	if err != nil || app.TenantID != teamID {
		httpError(w, http.StatusNotFound, "oauth app not found")
		return
	}
	if err := s.forgeOAuthApps.Delete(ctx, appID); err != nil {
		httpError(w, http.StatusInternalServerError, "delete oauth app: %v", err)
		return
	}
	s.auditTenant(r, teamID, "forge.oauth_app.deleted", "forge_oauth_app", appID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// createForgeOAuthApp seals the client_secret, persists the app row, and audits
// it. Shared by the manual-register handler and (later) the auto-create modes —
// the latter pass the client_id/client_secret they got back from the forge.
// Returns the stored app with SealedSecret nilled, ready to serialise.
func (s *Server) createForgeOAuthApp(r *http.Request, teamID, userID string, provider forge.Provider, rawBaseURL, clientID, clientSecret, providerAppID string, autoCreated bool, mode string) (forge.ForgeOAuthApp, error) {
	baseURL := forge.CanonicalBaseURL(provider, rawBaseURL)
	appID := uuid.NewString()
	sealed, err := forge.SealOAuthAppSecret(s.sealer, appID, clientSecret)
	if err != nil {
		return forge.ForgeOAuthApp{}, fmt.Errorf("seal secret: %w", err)
	}
	now := time.Now().UTC()
	app := forge.ForgeOAuthApp{
		ID: appID, TenantID: teamID, Provider: provider, ForgeBaseURL: baseURL,
		ClientID: clientID, SealedSecret: sealed, RedirectURI: s.forgeOAuthRedirectURI(),
		ProviderAppID: providerAppID, AutoCreated: autoCreated,
		CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.forgeOAuthApps.Create(store.WithTenant(r.Context(), teamID), app); err != nil {
		return forge.ForgeOAuthApp{}, err
	}
	s.auditTenant(r, teamID, "forge.oauth_app.created", "forge_oauth_app", appID, map[string]any{
		"provider": provider, "mode": mode, "instance": baseURL, "auto_created": autoCreated,
	})
	app.SealedSecret = nil
	return app, nil
}

// writeForgeOAuthAppError maps store / provider errors to HTTP responses,
// including the auto-create scope errors used in a later step.
func (s *Server) writeForgeOAuthAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, forge.ErrOAuthAppExists):
		httpError(w, http.StatusConflict, "%v", err)
	case errors.Is(err, forge.ErrForbidden):
		writeJSONStatus(w, http.StatusForbidden, map[string]any{
			"error":  "insufficient_scope",
			"detail": "the token can't create an OAuth app on this instance — GitLab needs an instance-admin token; or paste an existing client_id/client_secret instead",
		})
	case errors.Is(err, forge.ErrUnauthorized):
		httpError(w, http.StatusBadRequest, "the token was rejected by the forge")
	default:
		httpError(w, http.StatusInternalServerError, "%v", err)
	}
}
