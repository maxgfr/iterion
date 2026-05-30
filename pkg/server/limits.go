package server

import (
	"encoding/json"
	"net/http"
)

// registerLimitsRoutes wires the daily spend-cap status + override
// endpoints. No-op when no run store is configured (cloud control plane
// without a local store). Auth is provided by the global /api/*
// middleware wrap, mirroring registerRunsStatsRoutes.
func (s *Server) registerLimitsRoutes() {
	if s.runs == nil {
		return
	}
	s.mux.HandleFunc("GET /api/v1/limits/cost", s.handleCostCapStatus)
	s.mux.HandleFunc("POST /api/v1/limits/cost/override", s.handleCostCapOverride)
}

// handleCostCapStatus answers GET /api/v1/limits/cost with the current
// per-(store, UTC-day) spend-cap status. A disabled cap reports
// {"enabled": false}.
func (s *Server) handleCostCapStatus(w http.ResponseWriter, r *http.Request) {
	s.stateMu.RLock()
	runsSvc := s.runs
	s.stateMu.RUnlock()
	if runsSvc == nil {
		httpError(w, http.StatusServiceUnavailable, "no run store configured on this server")
		return
	}
	// DailyCap() may be nil (cap disabled); Status is nil-receiver-safe
	// and returns a disabled status.
	st, err := runsSvc.DailyCap().Status(r.Context())
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	s.writeJSONFor(w, r, st)
}

// costCapOverrideRequest is the POST body for the override endpoint.
// Active defaults to true (grant the override) when omitted, so a bare
// POST is the common "override for today" action; pass {"active": false}
// to revoke it. Note is an optional free-text audit reason.
type costCapOverrideRequest struct {
	Active *bool  `json:"active,omitempty"`
	Note   string `json:"note,omitempty"`
}

// handleCostCapOverride answers POST /api/v1/limits/cost/override. It
// sets (or clears) today's override flag — the one-click "override for
// today" the studio banner triggers. The grant is recorded in the day's
// ledger (granted_by / granted_at / note) as the audit trail, and
// auto-clears when the UTC day rolls over.
func (s *Server) handleCostCapOverride(w http.ResponseWriter, r *http.Request) {
	s.stateMu.RLock()
	runsSvc := s.runs
	s.stateMu.RUnlock()
	if runsSvc == nil {
		httpError(w, http.StatusServiceUnavailable, "no run store configured on this server")
		return
	}
	cap := runsSvc.DailyCap()
	if cap == nil {
		s.httpErrorFor(w, r, http.StatusConflict, "daily spend cap is not enabled on this server")
		return
	}

	var req costCapOverrideRequest
	if r.Body != nil {
		// Tolerate an empty body — a bare POST means "override for today".
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	st, err := cap.SetOverride(r.Context(), active, "operator", req.Note)
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	s.writeJSONFor(w, r, st)
}
