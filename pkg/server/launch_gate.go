package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
)

// OrgLimitDefaults are the platform-wide launch limits applied when a
// team doesn't carry its own override (Team field == 0). Zero means
// "no limit" — the safe default for existing deployments.
type OrgLimitDefaults struct {
	MonthlyRunQuota   int
	MonthlyCostCapUSD float64
	MaxConcurrentRuns int
	LaunchRatePerMin  int
}

// launchDenial is one launch-gate refusal: an HTTP status, a stable
// machine-readable reason token (the SPA and API clients switch on
// it), and a human detail. Quota denials carry the month-reset time;
// throttle denials carry a Retry-After hint.
type launchDenial struct {
	status     int
	reason     string
	detail     string
	retryAfter time.Duration
	resetAt    time.Time
}

// Stable denial reason tokens (API contract — documented in
// docs/quotas-and-limits.md).
const (
	denyOrgSuspended      = "org_suspended"
	denyMonthlyRunQuota   = "monthly_run_quota_exceeded"
	denyMonthlyCostCap    = "monthly_cost_cap_exceeded"
	denyConcurrencyCap    = "concurrency_cap_exceeded"
	denyLaunchRateLimited = "launch_rate_limited"
)

// activeRunCounter is the optional store capability the concurrency
// cap needs: how many of the org's runs are currently active
// (queued + running). The Mongo store implements it; the filesystem
// store doesn't — local mode is single-operator and has no per-org
// concurrency semantics.
type activeRunCounter interface {
	CountActiveRunsByTenant(ctx context.Context, tenantID string) (int, error)
}

// orValue returns the team override when set (> 0), else the platform
// default.
func orValue[T int | float64](team, def T) T {
	if team > 0 {
		return team
	}
	return def
}

// gateLaunch is the shared run-launch admission gate: suspend →
// concurrency → launch rate → monthly cost cap → monthly run quota
// (the last one is also the metering increment). Called by
// handleLaunchRun, handleResumeRun and the inbound webhook handlers.
//
// Fail-open on store errors, mirroring orgCanLaunch: quotas are an
// operator policy, not a hard security boundary — a transient Mongo
// blip must not wedge every launch. Super-admins bypass entirely.
// The run-quota increment is the one exception to fail-open being
// "free": when AllowRun errors the launch proceeds unmetered (logged).
func (s *Server) gateLaunch(ctx context.Context) *launchDenial {
	id, _ := auth.FromContext(ctx)
	st := s.authStore()
	if st == nil || id.IsSuperAdmin || id.TeamID == "" {
		return nil
	}
	t, err := st.GetTeam(ctx, id.TeamID)
	if err != nil {
		return nil // fail-open (see doc comment)
	}
	if !t.CanLaunch() {
		return &launchDenial{
			status: http.StatusForbidden,
			reason: denyOrgSuspended,
			detail: "org cannot launch runs (suspended or read-only)",
		}
	}
	now := time.Now().UTC()
	if d := s.gateConcurrency(ctx, t); d != nil {
		return d
	}
	if d := s.gateLaunchRate(t); d != nil {
		return d
	}
	if d := s.gateCostCap(ctx, t, now); d != nil {
		return d
	}
	return s.gateRunQuota(ctx, t, now)
}

func (s *Server) gateConcurrency(ctx context.Context, t identity.Team) *launchDenial {
	maxActive := orValue(t.MaxConcurrentRuns, s.orgDefaults.MaxConcurrentRuns)
	if maxActive <= 0 {
		return nil
	}
	counter, ok := s.cfg.Store.(activeRunCounter)
	if !ok {
		return nil
	}
	active, err := counter.CountActiveRunsByTenant(ctx, t.ID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("launch gate: active-run count for %s: %v (fail-open)", t.ID, err)
		}
		return nil
	}
	if active >= maxActive {
		return &launchDenial{
			status:     http.StatusTooManyRequests,
			reason:     denyConcurrencyCap,
			detail:     fmt.Sprintf("org has %d active runs (cap %d) — retry when one finishes", active, maxActive),
			retryAfter: 30 * time.Second,
		}
	}
	return nil
}

func (s *Server) gateLaunchRate(t identity.Team) *launchDenial {
	perMin := orValue(t.LaunchRatePerMin, s.orgDefaults.LaunchRatePerMin)
	if perMin <= 0 || s.authLimiter == nil {
		return nil
	}
	bucket := authBucketCfg{rate: float64(perMin) / 60.0, burst: float64(perMin)}
	if ok, retry := s.authLimiter.allow("orglaunch:"+t.ID, bucket); !ok {
		return &launchDenial{
			status:     http.StatusTooManyRequests,
			reason:     denyLaunchRateLimited,
			detail:     fmt.Sprintf("org launch rate cap (%d/min) exceeded", perMin),
			retryAfter: retry,
		}
	}
	return nil
}

func (s *Server) gateCostCap(ctx context.Context, t identity.Team, now time.Time) *launchDenial {
	capUSD := orValue(t.MonthlyCostCapUSD, s.orgDefaults.MonthlyCostCapUSD)
	if capUSD <= 0 || s.orgUsage == nil {
		return nil
	}
	u, err := s.orgUsage.Usage(ctx, t.ID, now)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("launch gate: usage read for %s: %v (fail-open)", t.ID, err)
		}
		return nil
	}
	if u.CostUSD >= capUSD {
		return &launchDenial{
			status:  http.StatusPaymentRequired,
			reason:  denyMonthlyCostCap,
			detail:  fmt.Sprintf("monthly LLM cost cap reached ($%.2f of $%.2f)", u.CostUSD, capUSD),
			resetAt: nextMonthStart(now),
		}
	}
	return nil
}

func (s *Server) gateRunQuota(ctx context.Context, t identity.Team, now time.Time) *launchDenial {
	if s.orgUsage == nil {
		return nil
	}
	max := orValue(t.MonthlyRunQuota, s.orgDefaults.MonthlyRunQuota)
	ok, err := s.orgUsage.AllowRun(ctx, t.ID, now, max)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("launch gate: run metering for %s: %v (fail-open, launch unmetered)", t.ID, err)
		}
		return nil
	}
	if !ok {
		return &launchDenial{
			status:  http.StatusPaymentRequired,
			reason:  denyMonthlyRunQuota,
			detail:  fmt.Sprintf("monthly run quota (%d) exhausted", max),
			resetAt: nextMonthStart(now),
		}
	}
	return nil
}

// nextMonthStart is when monthly quotas reset (first instant of the
// next UTC month).
func nextMonthStart(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
}

// writeLaunchDenial renders a denial: `error` carries the stable
// reason token (machine contract), `detail` the human message, plus
// Retry-After / reset_at when applicable.
func (s *Server) writeLaunchDenial(w http.ResponseWriter, r *http.Request, d *launchDenial) {
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.LaunchDeniedTotal.WithLabelValues(d.reason).Inc()
	}
	if d.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.retryAfter.Seconds())+1))
	}
	body := map[string]string{"error": d.reason, "detail": d.detail}
	if !d.resetAt.IsZero() {
		body["reset_at"] = d.resetAt.Format(time.RFC3339)
	}
	w.Header().Set("Content-Type", "application/json")
	s.reflectAllowedOrigin(w, r)
	w.WriteHeader(d.status)
	_ = json.NewEncoder(w).Encode(body)
}
