package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/webhooks"
)

// Reasonable defaults for a new webhook (the operator can override).
var defaultWebhookRate = webhooks.Rate{Rate: 1.0, Burst: 10} // 1 req/s sustained, burst 10

// defaultOrgMonthlyWebhookCalls caps accepted inbound deliveries per org
// per month unless a tighter per-webhook MonthlyCallLimit is set.
const defaultOrgMonthlyWebhookCalls = 10000

// registerWebhookRoutes wires the per-org webhook-token CRUD. Inbound
// delivery routes are registered per provider (registerGitLabWebhookRoute).
func (s *Server) registerWebhookRoutes() {
	if s.authLimiter == nil {
		s.authLimiter = newAuthRateLimiter()
	}
	s.mux.Handle("GET /api/teams/{id}/webhooks", s.requireAuth(http.HandlerFunc(s.handleListWebhooks)))
	s.mux.Handle("POST /api/teams/{id}/webhooks", s.requireAuth(http.HandlerFunc(s.handleCreateWebhook)))
	s.mux.Handle("GET /api/teams/{id}/webhooks/{webhook_id}", s.requireAuth(http.HandlerFunc(s.handleGetWebhook)))
	s.mux.Handle("PATCH /api/teams/{id}/webhooks/{webhook_id}", s.requireAuth(http.HandlerFunc(s.handleUpdateWebhook)))
	s.mux.Handle("DELETE /api/teams/{id}/webhooks/{webhook_id}", s.requireAuth(http.HandlerFunc(s.handleDeleteWebhook)))
	s.mux.Handle("POST /api/teams/{id}/webhooks/{webhook_id}/rotate", s.requireAuth(http.HandlerFunc(s.handleRotateWebhook)))
	s.mux.Handle("GET /api/teams/{id}/webhooks/{webhook_id}/deliveries", s.requireAuth(http.HandlerFunc(s.handleListWebhookDeliveries)))
}

type webhookConfigReq struct {
	Name             *string           `json:"name,omitempty"`
	Provider         *string           `json:"provider,omitempty"`
	Enabled          *bool             `json:"enabled,omitempty"`
	BotIDs           []string          `json:"bot_ids,omitempty"`
	WildcardBots     *bool             `json:"wildcard_bots,omitempty"`
	DefaultBotID     *string           `json:"default_bot_id,omitempty"`
	ProjectAllowlist []string          `json:"project_allowlist,omitempty"`
	EventAllowlist   []string          `json:"event_allowlist,omitempty"`
	RateLimit        *webhooks.Rate    `json:"rate_limit,omitempty"`
	MonthlyCallLimit *int              `json:"monthly_call_limit,omitempty"`
	LaunchVars       map[string]string `json:"launch_vars,omitempty"`
	KeyOverrides     map[string]string `json:"key_overrides,omitempty"`
}

type webhookWithToken struct {
	Config webhooks.Config `json:"config"`
	Token  string          `json:"token"`
}

// normalizeBotScope validates the bot-scope inputs: a wildcard scope
// must be explicit (wildcard_bots:true → BotIDs forced to ["*"]); a
// non-wildcard scope must list ≥1 non-"*" bot.
func normalizeBotScope(botIDs []string, wildcard bool) ([]string, bool, error) {
	if wildcard {
		return []string{"*"}, true, nil
	}
	if len(botIDs) == 0 {
		return nil, false, errors.New("bot_ids required (or set wildcard_bots:true)")
	}
	for _, b := range botIDs {
		if strings.TrimSpace(b) == "" {
			return nil, false, errors.New("empty bot id")
		}
		if b == "*" {
			return nil, false, errors.New(`a bare "*" bot scope requires wildcard_bots:true`)
		}
	}
	return botIDs, false, nil
}

// webhookForTenant fetches a config and asserts it belongs to teamID
// (defence in depth on top of the role gate). Returns ok=false having
// already written the response.
func (s *Server) webhookForTenant(w http.ResponseWriter, r *http.Request, teamID, webhookID string) (webhooks.Config, bool) {
	cfg, err := s.webhookConfigs.Get(r.Context(), webhookID)
	if err != nil || cfg.TenantID != teamID {
		httpError(w, http.StatusNotFound, "webhook not found")
		return webhooks.Config{}, false
	}
	return cfg, true
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	list, err := s.webhookConfigs.ListByTenant(r.Context(), teamID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	if list == nil {
		list = []webhooks.Config{}
	}
	writeJSON(w, struct {
		Webhooks []webhooks.Config `json:"webhooks"`
	}{Webhooks: list})
}

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	cfg, ok := s.webhookForTenant(w, r, teamID, r.PathValue("webhook_id"))
	if !ok {
		return
	}
	writeJSON(w, cfg)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	var req webhookConfigReq
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	if req.Name == nil || *req.Name == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	provider := webhooks.ProviderGitLab
	if req.Provider != nil && *req.Provider != "" {
		provider = webhooks.Provider(*req.Provider)
	}
	if provider != webhooks.ProviderGitLab {
		httpError(w, http.StatusBadRequest, "unsupported provider %q (gitlab only for now)", provider)
		return
	}
	wildcard := req.WildcardBots != nil && *req.WildcardBots
	botIDs, wildcard, err := normalizeBotScope(req.BotIDs, wildcard)
	if err != nil {
		httpError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}
	plaintext, hash, last4, fp, err := webhooks.MintToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "mint token: %v", err)
		return
	}
	now := time.Now().UTC()
	rate := defaultWebhookRate
	if req.RateLimit != nil {
		rate = *req.RateLimit
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfg := webhooks.Config{
		ID:               uuid.NewString(),
		TenantID:         teamID,
		Name:             *req.Name,
		Provider:         provider,
		Enabled:          enabled,
		TokenHash:        hash,
		TokenLast4:       last4,
		Fingerprint:      fp,
		BotIDs:           botIDs,
		WildcardBots:     wildcard,
		ProjectAllowlist: req.ProjectAllowlist,
		EventAllowlist:   req.EventAllowlist,
		RateLimit:        rate,
		LaunchVars:       req.LaunchVars,
		KeyOverrides:     req.KeyOverrides,
		CreatedBy:        id.UserID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if req.DefaultBotID != nil {
		cfg.DefaultBotID = *req.DefaultBotID
	}
	if req.MonthlyCallLimit != nil {
		cfg.MonthlyCallLimit = *req.MonthlyCallLimit
	}
	if err := s.validateKeyOverrides(r.Context(), teamID, cfg.KeyOverrides); err != nil {
		httpError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}
	if err := s.webhookConfigs.Create(r.Context(), cfg); err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	if wildcard && s.logger != nil {
		s.logger.Warn("webhooks: org %s created a wildcard-bot webhook %s (by %s)", teamID, cfg.ID, id.UserID)
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, webhookWithToken{Config: cfg, Token: plaintext})
}

// validateKeyOverrides rejects a webhook key override that references a BYOK
// key the webhook's tenant doesn't own, or whose provider doesn't match. The
// resolver is already tenant-scoped (a foreign key_id silently won't match in
// secrets.Resolve), so this is a fail-fast UX guard, not the security boundary.
func (s *Server) validateKeyOverrides(ctx context.Context, tenantID string, overrides map[string]string) error {
	if len(overrides) == 0 || s.apiKeys == nil {
		return nil
	}
	for prov, keyID := range overrides {
		k, err := s.apiKeys.Get(ctx, keyID)
		if err != nil {
			return fmt.Errorf("key_overrides[%s]: api key %q not found", prov, keyID)
		}
		if k.TenantID != tenantID {
			return fmt.Errorf("key_overrides[%s]: api key %q belongs to another org", prov, keyID)
		}
		if string(k.Provider) != prov {
			return fmt.Errorf("key_overrides[%s]: api key %q is for provider %q", prov, keyID, k.Provider)
		}
	}
	return nil
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	cfg, ok := s.webhookForTenant(w, r, teamID, r.PathValue("webhook_id"))
	if !ok {
		return
	}
	var req webhookConfigReq
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.DefaultBotID != nil {
		cfg.DefaultBotID = *req.DefaultBotID
	}
	if req.ProjectAllowlist != nil {
		cfg.ProjectAllowlist = req.ProjectAllowlist
	}
	if req.EventAllowlist != nil {
		cfg.EventAllowlist = req.EventAllowlist
	}
	if req.RateLimit != nil {
		cfg.RateLimit = *req.RateLimit
	}
	if req.MonthlyCallLimit != nil {
		cfg.MonthlyCallLimit = *req.MonthlyCallLimit
	}
	if req.LaunchVars != nil {
		cfg.LaunchVars = req.LaunchVars
	}
	if req.KeyOverrides != nil {
		if err := s.validateKeyOverrides(r.Context(), teamID, req.KeyOverrides); err != nil {
			httpError(w, http.StatusBadRequest, "%s", err.Error())
			return
		}
		cfg.KeyOverrides = req.KeyOverrides
	}
	// Re-normalise bot scope only when the caller touched it.
	if req.BotIDs != nil || req.WildcardBots != nil {
		wildcard := cfg.WildcardBots
		if req.WildcardBots != nil {
			wildcard = *req.WildcardBots
		}
		botIDs := cfg.BotIDs
		if req.BotIDs != nil {
			botIDs = req.BotIDs
		}
		newBotIDs, newWildcard, err := normalizeBotScope(botIDs, wildcard)
		if err != nil {
			httpError(w, http.StatusBadRequest, "%s", err.Error())
			return
		}
		cfg.BotIDs, cfg.WildcardBots = newBotIDs, newWildcard
	}
	cfg.UpdatedAt = time.Now().UTC()
	if err := s.webhookConfigs.Update(r.Context(), cfg); err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	writeJSON(w, cfg)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	if _, ok := s.webhookForTenant(w, r, teamID, r.PathValue("webhook_id")); !ok {
		return
	}
	if err := s.webhookConfigs.Delete(r.Context(), r.PathValue("webhook_id")); err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRotateWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canManageTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "admin or owner required")
		return
	}
	cfg, ok := s.webhookForTenant(w, r, teamID, r.PathValue("webhook_id"))
	if !ok {
		return
	}
	plaintext, hash, last4, fp, err := webhooks.MintToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "mint token: %v", err)
		return
	}
	now := time.Now().UTC()
	cfg.TokenHash, cfg.TokenLast4, cfg.Fingerprint = hash, last4, fp
	cfg.RotatedAt = &now
	cfg.UpdatedAt = now
	if err := s.webhookConfigs.Update(r.Context(), cfg); err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	writeJSON(w, webhookWithToken{Config: cfg, Token: plaintext})
}

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member")
		return
	}
	if _, ok := s.webhookForTenant(w, r, teamID, r.PathValue("webhook_id")); !ok {
		return
	}
	if s.webhookDeliveries == nil {
		writeJSON(w, struct {
			Deliveries []webhooks.Delivery `json:"deliveries"`
		}{Deliveries: []webhooks.Delivery{}})
		return
	}
	list, err := s.webhookDeliveries.ListByWebhook(r.Context(), teamID, r.PathValue("webhook_id"), 100)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	if list == nil {
		list = []webhooks.Delivery{}
	}
	writeJSON(w, struct {
		Deliveries []webhooks.Delivery `json:"deliveries"`
	}{Deliveries: list})
}
