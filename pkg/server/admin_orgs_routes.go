package server

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/store"
)

// registerAdminOrgRoutes wires the super-admin org (team) console.
// "org" is the public-API alias for the internal Team/tenant — no
// storage rename. Every route is super-admin only.
func (s *Server) registerAdminOrgRoutes() {
	s.mux.Handle("GET /api/admin/orgs", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminListOrgs)))
	s.mux.Handle("POST /api/admin/orgs", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminCreateOrg)))
	s.mux.Handle("GET /api/admin/orgs/{id}", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminGetOrg)))
	s.mux.Handle("PATCH /api/admin/orgs/{id}", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminUpdateOrg)))
	s.mux.Handle("POST /api/admin/orgs/{id}/status", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminSetOrgStatus)))
	s.mux.Handle("GET /api/admin/orgs/{id}/usage", s.requireSuperAdmin(http.HandlerFunc(s.handleAdminOrgUsage)))
	// Org-admin self-serve mirror of the usage view (any member can
	// read their own org's consumption).
	s.mux.Handle("GET /api/teams/{id}/usage", s.requireAuth(http.HandlerFunc(s.handleTeamUsage)))
}

// ---- views / requests ----

type orgView struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Slug              string  `json:"slug"`
	Status            string  `json:"status"`
	Personal          bool    `json:"personal,omitempty"`
	MonthlyRunQuota   int     `json:"monthly_run_quota,omitempty"`
	MemoryQuotaBytes  int64   `json:"memory_quota_bytes,omitempty"`
	MonthlyCostCapUSD float64 `json:"monthly_cost_cap_usd,omitempty"`
	MaxConcurrentRuns int     `json:"max_concurrent_runs,omitempty"`
	LaunchRatePerMin  int     `json:"launch_rate_per_min,omitempty"`
	SuspendReason     string  `json:"suspend_reason,omitempty"`
	CreatedAt         string  `json:"created_at,omitempty"`
}

func toOrgView(t identity.Team) orgView {
	return orgView{
		ID:                t.ID,
		Name:              t.Name,
		Slug:              t.Slug,
		Status:            string(t.EffectiveStatus()),
		Personal:          t.Personal,
		MonthlyRunQuota:   t.MonthlyRunQuota,
		MemoryQuotaBytes:  t.MemoryQuotaBytes,
		MonthlyCostCapUSD: t.MonthlyCostCapUSD,
		MaxConcurrentRuns: t.MaxConcurrentRuns,
		LaunchRatePerMin:  t.LaunchRatePerMin,
		SuspendReason:     t.SuspendReason,
		CreatedAt:         t.CreatedAt.Format(time.RFC3339),
	}
}

// orgUsageView is the consumption snapshot for one org — served to
// super-admins (/api/admin/orgs/{id}/usage) and to the org's own
// members (/api/teams/{id}/usage). Counter-backed fields read zero
// when the corresponding store isn't wired (local mode).
type orgUsageView struct {
	Org     orgView `json:"org"`
	Members int     `json:"members"`
	// EffectiveMemoryQuotaBytes resolves the org override (or the
	// platform default) so the console shows the real ceiling.
	EffectiveMemoryQuotaBytes int64 `json:"effective_memory_quota_bytes"`
	MonthlyRunQuota           int   `json:"monthly_run_quota"`

	// Current-month metering (orgusage counter).
	RunsThisMonth    int     `json:"runs_this_month"`
	CostUSDThisMonth float64 `json:"cost_usd_this_month"`
	InputTokens      int64   `json:"input_tokens_this_month"`
	OutputTokens     int64   `json:"output_tokens_this_month"`
	// Caps as enforced by the launch gate (org override or platform
	// default; 0 = unlimited).
	MonthlyCostCapUSD float64 `json:"monthly_cost_cap_usd,omitempty"`
	MaxConcurrentRuns int     `json:"max_concurrent_runs,omitempty"`

	// Live + auxiliary counters.
	ActiveRuns            int   `json:"active_runs"`
	WebhookCallsThisMonth int   `json:"webhook_calls_this_month"`
	MemoryUsedBytes       int64 `json:"memory_used_bytes"`
	APIKeyCount           int   `json:"api_key_count"`
	GenericSecretCount    int   `json:"generic_secret_count"`
	BotBindingCount       int   `json:"bot_binding_count"`
	WebhookCount          int   `json:"webhook_count"`
}

type createOrgReq struct {
	Name       string `json:"name"`
	Slug       string `json:"slug,omitempty"`
	OwnerEmail string `json:"owner_email,omitempty"`
}

type updateOrgReq struct {
	Name              *string  `json:"name,omitempty"`
	Slug              *string  `json:"slug,omitempty"`
	MonthlyRunQuota   *int     `json:"monthly_run_quota,omitempty"`
	MemoryQuotaBytes  *int64   `json:"memory_quota_bytes,omitempty"`
	MonthlyCostCapUSD *float64 `json:"monthly_cost_cap_usd,omitempty"`
	MaxConcurrentRuns *int     `json:"max_concurrent_runs,omitempty"`
	LaunchRatePerMin  *int     `json:"launch_rate_per_min,omitempty"`
}

type setOrgStatusReq struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// effectiveOrgMemoryQuota resolves the org's memory ceiling: the
// explicit per-org override when set, else the platform default.
func effectiveOrgMemoryQuota(t identity.Team) int64 {
	if t.MemoryQuotaBytes > 0 {
		return t.MemoryQuotaBytes
	}
	return knowledge.DefaultOrgAggregateQuota
}

// authStoreOrFail returns the identity store, writing a 500 and
// reporting ok=false when it isn't wired (so super-admin handlers don't
// each repeat the nil check).
func (s *Server) authStoreOrFail(w http.ResponseWriter) (identity.Store, bool) {
	st := s.authStore()
	if st == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
		return nil, false
	}
	return st, true
}

// applyNonNegative copies *p into *dst when p is non-nil, rejecting
// negative values with a 400. Returns true on success (including the
// p==nil no-op), false if it already wrote an error response. Keeps the
// per-field error message identical to the hand-written form
// ("<field> must be >= 0") so the API surface is unchanged.
func applyNonNegative[T int | int64 | float64](w http.ResponseWriter, p *T, dst *T, field string) bool {
	if p == nil {
		return true
	}
	if *p < 0 {
		httpError(w, http.StatusBadRequest, "%s must be >= 0", field)
		return false
	}
	*dst = *p
	return true
}

// ---- handlers ----

func (s *Server) handleAdminListOrgs(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	teams, err := store.ListTeams(r.Context(), identity.Page{Limit: 500})
	if err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	views := make([]orgView, 0, len(teams))
	for _, t := range teams {
		views = append(views, toOrgView(t))
	}
	writeJSON(w, struct {
		Orgs []orgView `json:"orgs"`
	}{Orgs: views})
}

func (s *Server) handleAdminCreateOrg(w http.ResponseWriter, r *http.Request) {
	if s.authSvc == nil {
		httpError(w, http.StatusInternalServerError, "auth not configured")
		return
	}
	id, _ := auth.FromContext(r.Context())
	var req createOrgReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	ownerID := id.UserID
	if req.OwnerEmail != "" {
		u, err := s.authStore().GetUserByEmail(r.Context(), req.OwnerEmail)
		if err != nil {
			httpError(w, mapAuthErrorStatus(err), "owner: %s", err.Error())
			return
		}
		ownerID = u.ID
	}
	t, err := s.authSvc.CreateTeamFor(r.Context(), ownerID, req.Name, req.Slug)
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	s.auditPlatform(r, t.ID, "org.created", "org", t.ID, map[string]any{"name": t.Name, "owner": ownerID})
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminGetOrg(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminUpdateOrg(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	var req updateOrgReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Slug != nil {
		t.Slug = *req.Slug
	}
	if !applyNonNegative(w, req.MonthlyRunQuota, &t.MonthlyRunQuota, "monthly_run_quota") {
		return
	}
	if !applyNonNegative(w, req.MemoryQuotaBytes, &t.MemoryQuotaBytes, "memory_quota_bytes") {
		return
	}
	if !applyNonNegative(w, req.MonthlyCostCapUSD, &t.MonthlyCostCapUSD, "monthly_cost_cap_usd") {
		return
	}
	if !applyNonNegative(w, req.MaxConcurrentRuns, &t.MaxConcurrentRuns, "max_concurrent_runs") {
		return
	}
	if !applyNonNegative(w, req.LaunchRatePerMin, &t.LaunchRatePerMin, "launch_rate_per_min") {
		return
	}
	t.UpdatedAt = time.Now().UTC()
	if err := store.UpdateTeam(r.Context(), t); err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	// Propagate a memory-quota change to the counter the CAS actually
	// enforces. Persisting Team.MemoryQuotaBytes alone had no effect —
	// the Mongo memory store seeds tenant quota from the platform default
	// and never re-read the Team. No-op for the FS store (local mode has
	// no per-tenant memory quota).
	if req.MemoryQuotaBytes != nil {
		if setter, ok := s.memoryStore().(tenantMemoryQuotaSetter); ok {
			if err := setter.SetTenantQuota(r.Context(), t.ID, effectiveOrgMemoryQuota(t)); err != nil && s.logger != nil {
				s.logger.Warn("admin: propagate memory quota for org %s: %v", t.ID, err)
			}
		}
	}
	s.auditPlatform(r, t.ID, "org.updated", "org", t.ID, map[string]any{
		"monthly_run_quota": t.MonthlyRunQuota, "memory_quota_bytes": t.MemoryQuotaBytes,
		"monthly_cost_cap_usd": t.MonthlyCostCapUSD, "max_concurrent_runs": t.MaxConcurrentRuns,
	})
	writeJSON(w, toOrgView(t))
}

// tenantMemoryQuotaSetter is the capability the cloud (Mongo) memory
// store implements so the org console can push a quota change onto the
// enforced counter. The FS store does not implement it.
type tenantMemoryQuotaSetter interface {
	SetTenantQuota(ctx context.Context, tenantID string, quotaBytes int64) error
}

func (s *Server) handleAdminSetOrgStatus(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	id, _ := auth.FromContext(r.Context())
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	var req setOrgStatusReq
	if !decodeJSON(w, r, &req) {
		return
	}
	st := identity.TeamStatus(req.Status)
	if !identity.ValidTeamStatus(st) {
		httpError(w, http.StatusBadRequest, "invalid status (active|suspended|read_only)")
		return
	}
	t.Status = st
	if st == identity.TeamStatusSuspended {
		now := time.Now().UTC()
		t.SuspendedAt = &now
		t.SuspendedBy = id.UserID
		t.SuspendReason = req.Reason
	} else {
		t.SuspendedAt = nil
		t.SuspendedBy = ""
		t.SuspendReason = ""
	}
	t.UpdatedAt = time.Now().UTC()
	if err := store.UpdateTeam(r.Context(), t); err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	if s.logger != nil {
		s.logger.Info("admin: org %s status -> %s by %s", t.ID, st, id.UserID)
	}
	s.auditPlatform(r, t.ID, "org.status_changed", "org", t.ID, map[string]any{"status": string(st), "reason": req.Reason})
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminOrgUsage(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	writeJSON(w, s.buildOrgUsageView(r.Context(), store, t))
}

// handleTeamUsage is the org-admin self-serve mirror: any member of
// the team can read its consumption (writes stay admin-gated).
func (s *Server) handleTeamUsage(w http.ResponseWriter, r *http.Request) {
	store, ok := s.authStoreOrFail(w)
	if !ok {
		return
	}
	id, _ := auth.FromContext(r.Context())
	teamID := r.PathValue("id")
	if !s.canViewTeam(r.Context(), id, teamID) {
		httpError(w, http.StatusForbidden, "not a member of this team")
		return
	}
	t, err := store.GetTeam(r.Context(), teamID)
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	writeJSON(w, s.buildOrgUsageView(r.Context(), store, t))
}

// tenantMemoryUsageReader is the capability the cloud memory store
// implements for the org-aggregate consumption readout. The FS store
// doesn't (local mode has no per-tenant aggregate).
type tenantMemoryUsageReader interface {
	TenantUsedBytes(ctx context.Context, tenantID string) (int64, error)
}

// buildOrgUsageView assembles the usage snapshot from every wired
// store. Each sub-read is best-effort: a missing store or a transient
// error leaves its field at zero rather than failing the whole view.
//
// The ctx is re-stamped onto the TARGET org before the tenant-scoped
// reads: the caller's ctx carries their ACTIVE team (super-admin
// inspecting org X, or a member whose active team is a sibling), and
// the secrets stores' ctx tenant filter would otherwise silently
// zero every count. Authorization happened in the handlers; this is
// scoping, not privilege.
func (s *Server) buildOrgUsageView(ctx context.Context, st identity.Store, t identity.Team) orgUsageView {
	id, _ := auth.FromContext(ctx)
	ctx = store.WithIdentity(ctx, t.ID, id.UserID)
	members, _ := st.ListMembershipsByTeam(ctx, t.ID)
	v := orgUsageView{
		Org:                       toOrgView(t),
		Members:                   len(members),
		EffectiveMemoryQuotaBytes: effectiveOrgMemoryQuota(t),
		MonthlyRunQuota:           orValue(t.MonthlyRunQuota, s.orgDefaults.MonthlyRunQuota),
		MonthlyCostCapUSD:         orValue(t.MonthlyCostCapUSD, s.orgDefaults.MonthlyCostCapUSD),
		MaxConcurrentRuns:         orValue(t.MaxConcurrentRuns, s.orgDefaults.MaxConcurrentRuns),
	}
	now := time.Now().UTC()

	// Fan out the ~8 independent best-effort reads. Each goroutine
	// writes its OWN distinct field of `v` (no shared field → no
	// mutex needed); transient errors still just leave a zero value
	// rather than failing the whole view, so we never propagate them
	// through the errgroup (Wait's return value is intentionally
	// discarded).
	g, gctx := errgroup.WithContext(ctx)
	if s.orgUsage != nil {
		g.Go(func() error {
			if u, err := s.orgUsage.Usage(gctx, t.ID, now); err == nil {
				v.RunsThisMonth = u.Runs
				v.CostUSDThisMonth = u.CostUSD
				v.InputTokens = u.InputTokens
				v.OutputTokens = u.OutputTokens
			}
			return nil
		})
	}
	if s.webhookCounter != nil {
		g.Go(func() error {
			if n, err := s.webhookCounter.OrgCount(gctx, t.ID, now); err == nil {
				v.WebhookCallsThisMonth = n
			}
			return nil
		})
	}
	if counter, ok := s.cfg.Store.(activeRunCounter); ok {
		g.Go(func() error {
			if n, err := counter.CountActiveRunsByTenant(gctx, t.ID); err == nil {
				v.ActiveRuns = n
			}
			return nil
		})
	}
	if reader, ok := s.memoryStore().(tenantMemoryUsageReader); ok {
		g.Go(func() error {
			if n, err := reader.TenantUsedBytes(gctx, t.ID); err == nil {
				v.MemoryUsedBytes = n
			}
			return nil
		})
	}
	if s.apiKeys != nil {
		g.Go(func() error {
			// "" requesting user → team-wide keys only (the admin path
			// documented on ApiKeyStore.ListByTeam).
			if keys, err := s.apiKeys.ListByTeam(gctx, t.ID, ""); err == nil {
				v.APIKeyCount = len(keys)
			}
			return nil
		})
	}
	if s.genericSecrets != nil {
		g.Go(func() error {
			if secs, err := s.genericSecrets.ListByTeam(gctx, t.ID, ""); err == nil {
				v.GenericSecretCount = len(secs)
			}
			return nil
		})
	}
	if s.botBindings != nil {
		g.Go(func() error {
			if bs, err := s.botBindings.ListByTenant(gctx, t.ID); err == nil {
				v.BotBindingCount = len(bs)
			}
			return nil
		})
	}
	if s.webhookConfigs != nil {
		g.Go(func() error {
			if whs, err := s.webhookConfigs.ListByTenant(gctx, t.ID); err == nil {
				v.WebhookCount = len(whs)
			}
			return nil
		})
	}
	_ = g.Wait()
	return v
}

// ---- launch suspend gate ----

// orgCanLaunch is the suspend-only gate decision, isolated for
// testability. The full launch admission (quotas, concurrency, rate)
// lives in gateLaunch (launch_gate.go), which folds this check in. It
// returns true (allow) when there is no identity store (local mode),
// the caller is a super-admin, has no active team, or the team lookup
// fails (fail-open: suspension is an operator action, not a hard
// security boundary — a transient store error must not wedge launches).
func orgCanLaunch(ctx context.Context, st identity.Store, id auth.Identity) bool {
	if st == nil || id.IsSuperAdmin || id.TeamID == "" {
		return true
	}
	t, err := st.GetTeam(ctx, id.TeamID)
	if err != nil {
		return true
	}
	return t.CanLaunch()
}
