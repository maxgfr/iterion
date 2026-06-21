package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/bundle"
	"github.com/SocialGouv/iterion/pkg/forge"
	"github.com/SocialGouv/iterion/pkg/store"
)

func (s *Server) registerForgeProvisioningRoutes() {
	s.mux.Handle("GET /api/teams/{id}/forge/repo-bots", s.requireAuth(http.HandlerFunc(s.handleListForgeRepoBots)))
	s.mux.Handle("POST /api/teams/{id}/forge/repo-bots", s.requireAuth(http.HandlerFunc(s.handleEnableForgeRepoBots)))
	s.mux.Handle("GET /api/teams/{id}/forge/repo-bots/preview", s.requireAuth(http.HandlerFunc(s.handlePreviewForgeEnable)))
	s.mux.Handle("DELETE /api/teams/{id}/forge/repo-bots/{integration_id}", s.requireAuth(http.HandlerFunc(s.handleDisableForgeRepoBots)))
}

func (s *Server) handleListForgeRepoBots(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	list, err := s.forgeIntegrations.ListByTenant(r.Context(), teamID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	if list == nil {
		list = []forge.RepoIntegration{}
	}
	writeJSON(w, struct {
		Integrations []forge.RepoIntegration `json:"integrations"`
	}{Integrations: list})
}

type forgeEnableReq struct {
	ConnectionID string   `json:"connection_id"`
	Repo         string   `json:"repo"`
	BotIDs       []string `json:"bot_ids"`
}

func (s *Server) handleEnableForgeRepoBots(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	var req forgeEnableReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ConnectionID) == "" || strings.TrimSpace(req.Repo) == "" || len(req.BotIDs) == 0 {
		httpError(w, http.StatusBadRequest, "connection_id, repo and bot_ids are required")
		return
	}
	// Assert the connection belongs to this team before we provision.
	if _, ok := s.forgeConnForTenant(w, r, teamID, req.ConnectionID); !ok {
		return
	}
	ctx := store.WithTenant(r.Context(), teamID)
	res, err := s.forgeOrchestrator.Provision(ctx, forge.ProvisionRequest{
		TenantID:     teamID,
		ConnectionID: req.ConnectionID,
		RepoFullName: strings.TrimSpace(req.Repo),
		BotIDs:       req.BotIDs,
		ActorID:      id.UserID,
	})
	if err != nil {
		s.writeForgeProvisionError(w, err)
		return
	}
	s.auditTenant(r, teamID, "forge.integration.provisioned", "forge_integration", res.IntegrationID, map[string]any{
		"repo": req.Repo, "bots": res.BotIDs, "connection_id": req.ConnectionID,
	})
	writeJSON(w, res)
}

func (s *Server) handleDisableForgeRepoBots(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	integrationID := r.PathValue("integration_id")
	ctx := store.WithTenant(r.Context(), teamID)
	if err := s.forgeOrchestrator.Deprovision(ctx, teamID, integrationID); err != nil {
		if errors.Is(err, forge.ErrIntegrationNotFound) {
			httpError(w, http.StatusNotFound, "integration not found")
			return
		}
		s.writeForgeProvisionError(w, err)
		return
	}
	s.auditTenant(r, teamID, "forge.integration.deprovisioned", "forge_integration", integrationID, nil)
	w.WriteHeader(http.StatusNoContent)
}

type forgeEnablePreview struct {
	EventsNormalized  []string           `json:"events_normalized"`
	ForgeNativeEvents []string           `json:"forge_native_events"`
	Scopes            map[string]string  `json:"scopes"`
	Secrets           []forgePreviewBind `json:"secrets"`
	Identity          forgePreviewIdent  `json:"identity"`
	Conflicts         []string           `json:"conflicts"`
}

type forgePreviewBind struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

type forgePreviewIdent struct {
	Handle   string `json:"handle"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
}

// handlePreviewForgeEnable computes exactly what enabling a set of bots on a
// repo will subscribe to + request — read-only, no forge writes — so the
// studio can show "Revi will subscribe to … and post as …" before the
// operator commits.
func (s *Server) handlePreviewForgeEnable(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	conn, ok := s.forgeConnForTenant(w, r, teamID, r.URL.Query().Get("connection_id"))
	if !ok {
		return
	}
	botIDs := splitCSV(r.URL.Query().Get("bots"))
	if len(botIDs) == 0 {
		httpError(w, http.StatusBadRequest, "bots query param required")
		return
	}

	var reqs []*bundle.ForgeRequirements
	var conflicts []string
	binds := make([]forgePreviewBind, 0, len(botIDs))
	for _, b := range botIDs {
		fr, err := s.forgeOrchestrator.Bots(b)
		if err != nil {
			conflicts = append(conflicts, b+": "+err.Error())
			continue
		}
		if fr == nil {
			conflicts = append(conflicts, b+": declares no forge: block — not auto-installable")
			continue
		}
		reqs = append(reqs, fr)
		binds = append(binds, forgePreviewBind{BotID: b, Secret: fr.SecretName()})
	}
	events := forge.UnionEvents(reqs...)
	writeJSON(w, forgeEnablePreview{
		EventsNormalized:  events,
		ForgeNativeEvents: forge.ToNativeEvents(conn.Provider, events),
		Scopes:            forge.UnionScopes(reqs...),
		Secrets:           binds,
		Identity:          forgePreviewIdent{Handle: conn.AccountLogin, Provider: string(conn.Provider), BaseURL: conn.BaseURL()},
		Conflicts:         conflicts,
	})
}

// writeForgeProvisionError maps orchestrator/admin failures to stable HTTP
// shapes the studio can act on.
func (s *Server) writeForgeProvisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, forge.ErrForbidden):
		writeJSONStatus(w, http.StatusForbidden, map[string]any{
			"error":  "insufficient_scope",
			"detail": "the connection's token cannot manage webhooks on this repo — reconnect with broader scope or paste a PAT with hook-admin rights",
		})
	case errors.Is(err, forge.ErrUnauthorized):
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":  "connection_unauthorized",
			"detail": "the connection credential was rejected — reconnect",
		})
	case errors.Is(err, forge.ErrConnectionNotFound):
		httpError(w, http.StatusNotFound, "connection not found")
	default:
		httpError(w, http.StatusBadGateway, "provisioning failed: %v", err)
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
