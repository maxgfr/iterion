package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/knowledge"
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
}

// ---- views / requests ----

type orgView struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Status           string `json:"status"`
	Personal         bool   `json:"personal,omitempty"`
	MonthlyRunQuota  int    `json:"monthly_run_quota,omitempty"`
	MemoryQuotaBytes int64  `json:"memory_quota_bytes,omitempty"`
	SuspendReason    string `json:"suspend_reason,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
}

func toOrgView(t identity.Team) orgView {
	return orgView{
		ID:               t.ID,
		Name:             t.Name,
		Slug:             t.Slug,
		Status:           string(t.EffectiveStatus()),
		Personal:         t.Personal,
		MonthlyRunQuota:  t.MonthlyRunQuota,
		MemoryQuotaBytes: t.MemoryQuotaBytes,
		SuspendReason:    t.SuspendReason,
		CreatedAt:        t.CreatedAt.Format(time.RFC3339),
	}
}

type orgUsageView struct {
	Org     orgView `json:"org"`
	Members int     `json:"members"`
	// EffectiveMemoryQuotaBytes resolves the org override (or the
	// platform default) so the console shows the real ceiling.
	EffectiveMemoryQuotaBytes int64 `json:"effective_memory_quota_bytes"`
	MonthlyRunQuota           int   `json:"monthly_run_quota"`
	// TODO(phases 3/5/6): runs this month, webhook calls, memory used
	// bytes, secret/BYOK counts — wired in as those stores land.
}

type createOrgReq struct {
	Name       string `json:"name"`
	Slug       string `json:"slug,omitempty"`
	OwnerEmail string `json:"owner_email,omitempty"`
}

type updateOrgReq struct {
	Name             *string `json:"name,omitempty"`
	Slug             *string `json:"slug,omitempty"`
	MonthlyRunQuota  *int    `json:"monthly_run_quota,omitempty"`
	MemoryQuotaBytes *int64  `json:"memory_quota_bytes,omitempty"`
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

// ---- handlers ----

func (s *Server) handleAdminListOrgs(w http.ResponseWriter, r *http.Request) {
	store := s.authStore()
	if store == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
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
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
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
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminGetOrg(w http.ResponseWriter, r *http.Request) {
	store := s.authStore()
	if store == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
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
	store := s.authStore()
	if store == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
		return
	}
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	var req updateOrgReq
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Slug != nil {
		t.Slug = *req.Slug
	}
	if req.MonthlyRunQuota != nil {
		if *req.MonthlyRunQuota < 0 {
			httpError(w, http.StatusBadRequest, "monthly_run_quota must be >= 0")
			return
		}
		t.MonthlyRunQuota = *req.MonthlyRunQuota
	}
	if req.MemoryQuotaBytes != nil {
		if *req.MemoryQuotaBytes < 0 {
			httpError(w, http.StatusBadRequest, "memory_quota_bytes must be >= 0")
			return
		}
		t.MemoryQuotaBytes = *req.MemoryQuotaBytes
	}
	t.UpdatedAt = time.Now().UTC()
	if err := store.UpdateTeam(r.Context(), t); err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminSetOrgStatus(w http.ResponseWriter, r *http.Request) {
	store := s.authStore()
	if store == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
		return
	}
	id, _ := auth.FromContext(r.Context())
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	var req setOrgStatusReq
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
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
	writeJSON(w, toOrgView(t))
}

func (s *Server) handleAdminOrgUsage(w http.ResponseWriter, r *http.Request) {
	store := s.authStore()
	if store == nil {
		httpError(w, http.StatusInternalServerError, "identity store unavailable")
		return
	}
	t, err := store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	members, _ := store.ListMembershipsByTeam(r.Context(), t.ID)
	writeJSON(w, orgUsageView{
		Org:                       toOrgView(t),
		Members:                   len(members),
		EffectiveMemoryQuotaBytes: effectiveOrgMemoryQuota(t),
		MonthlyRunQuota:           t.MonthlyRunQuota,
	})
}

// ---- launch suspend gate ----

// errTeamCannotLaunch signals the caller's active team is suspended or
// read-only.
var errTeamCannotLaunch = errors.New("team cannot launch runs")

// teamLaunchGate denies a run launch when the caller's active team is
// suspended/read-only.
func (s *Server) teamLaunchGate(ctx context.Context) error {
	id, _ := auth.FromContext(ctx)
	if orgCanLaunch(ctx, s.authStore(), id) {
		return nil
	}
	return errTeamCannotLaunch
}

// orgCanLaunch is the gate decision, isolated for testability. It
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
