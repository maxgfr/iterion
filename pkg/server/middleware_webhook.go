package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/SocialGouv/iterion/pkg/webhooks"
)

type webhookCtxKey struct{}

// webhookConfigFromContext returns the authenticated webhook Config a
// provider handler should act on. Set by webhookAuth.
func webhookConfigFromContext(ctx context.Context) (webhooks.Config, bool) {
	c, ok := ctx.Value(webhookCtxKey{}).(webhooks.Config)
	return c, ok
}

// extractWebhookToken pulls the presented token from the provider's
// native header, falling back to iterion's own header.
func extractWebhookToken(r *http.Request, provider webhooks.Provider) string {
	if provider == webhooks.ProviderGitLab {
		if t := r.Header.Get("X-Gitlab-Token"); t != "" {
			return t
		}
	}
	return r.Header.Get("X-Iterion-Webhook-Token")
}

// webhookAuth is the inbound-webhook authentication + admission
// middleware. It is NOT chained from requireAuth — /api/webhooks/* is
// public to the JWT layer (see isPublicPath) and authenticates itself.
//
// Order (everything before the handler does any work): resolve config by
// URL id → constant-time token check → enabled → per-webhook rate limit
// → per-org monthly quota → org not suspended → stamp tenant identity +
// the config on ctx. A failure at any step short-circuits with the right
// status and never reaches the provider handler.
func (s *Server) webhookAuth(provider webhooks.Provider, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.webhookConfigs == nil {
			httpError(w, http.StatusNotFound, "webhooks not enabled")
			return
		}
		id := r.PathValue("id")
		if id == "" {
			httpError(w, http.StatusBadRequest, "webhook id required")
			return
		}
		// Resolved without a tenant context — the webhook IS the tenant
		// selector. Collapse not-found and provider-mismatch into the
		// same 401 as a bad token so the id space isn't probeable.
		cfg, err := s.webhookConfigs.Get(r.Context(), id)
		if err != nil || cfg.Provider != provider {
			httpError(w, http.StatusUnauthorized, "invalid webhook")
			return
		}
		if !webhooks.VerifyToken(extractWebhookToken(r, provider), cfg.TokenHash) {
			if s.logger != nil {
				s.logger.Warn("webhooks: bad token for %s from %s", cfg.ID, s.clientIP(r))
			}
			httpError(w, http.StatusUnauthorized, "invalid webhook token")
			return
		}
		if !cfg.Enabled {
			httpError(w, http.StatusGone, "webhook disabled")
			return
		}
		// Per-webhook rate limit.
		if s.authLimiter != nil {
			bucket := authBucketCfg{rate: cfg.RateLimit.Rate, burst: cfg.RateLimit.Burst}
			if bucket.rate <= 0 || bucket.burst <= 0 {
				bucket = authBucketCfg{rate: defaultWebhookRate.Rate, burst: defaultWebhookRate.Burst}
			}
			if ok, retry := s.authLimiter.allow("wh:"+cfg.ID, bucket); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				httpError(w, http.StatusTooManyRequests, "rate limited")
				return
			}
		}
		// Per-org monthly quota (and optional per-webhook monthly cap).
		if s.webhookCounter != nil {
			lim := webhooks.Limits{
				PerOrgMonthly:     defaultOrgMonthlyWebhookCalls,
				PerWebhookMonthly: cfg.MonthlyCallLimit,
			}
			ok, qerr := s.webhookCounter.Allow(r.Context(), cfg.TenantID, cfg.ID, time.Now().UTC(), lim)
			if qerr != nil {
				httpError(w, http.StatusInternalServerError, "quota check failed")
				return
			}
			if !ok {
				httpError(w, http.StatusTooManyRequests, "monthly call quota exceeded")
				return
			}
		}
		// Org must be active.
		if st := s.authStore(); st != nil {
			if t, terr := st.GetTeam(r.Context(), cfg.TenantID); terr == nil && !t.CanLaunch() {
				httpError(w, http.StatusForbidden, "org suspended")
				return
			}
		}
		// Stamp the tenant identity (synthetic webhook actor) + config.
		actor := "webhook:" + cfg.ID
		ctx := auth.WithIdentity(r.Context(), auth.Identity{
			UserID: actor,
			TeamID: cfg.TenantID,
			Role:   identity.RoleMember,
		})
		ctx = store.WithIdentity(ctx, cfg.TenantID, actor)
		ctx = context.WithValue(ctx, webhookCtxKey{}, cfg)
		_ = s.webhookConfigs.MarkUsed(ctx, cfg.ID, time.Now().UTC())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
