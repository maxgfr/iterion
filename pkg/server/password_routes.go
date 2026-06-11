package server

import (
	"net/http"

	"github.com/SocialGouv/iterion/pkg/auth"
)

// handlePasswordResetRequest mints + emails a one-shot reset link.
// ALWAYS 200 with the same body — account enumeration through this
// endpoint must be impossible (the service logs the real outcome).
func (s *Server) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if s.authSvc == nil {
		httpError(w, http.StatusServiceUnavailable, "auth not configured")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil || req.Email == "" {
		httpError(w, http.StatusBadRequest, "email required")
		return
	}
	_ = s.authSvc.RequestPasswordReset(r.Context(), req.Email)
	writeJSON(w, map[string]string{"status": "ok"})
}

// handlePasswordResetConfirm redeems the emailed token: new password,
// every prior session revoked, fresh login issued.
func (s *Server) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if s.authSvc == nil {
		httpError(w, http.StatusServiceUnavailable, "auth not configured")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil || req.Token == "" || req.Password == "" {
		httpError(w, http.StatusBadRequest, "token + password required")
		return
	}
	res, err := s.authSvc.ConfirmPasswordReset(r.Context(), req.Token, req.Password, r.UserAgent(), s.clientIP(r))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	s.auditPlatform(r, res.ActiveTeamID, "user.password_reset", "user", res.User.ID, nil)
	s.renderAuthResponse(w, r, res)
}

// handleChangeMyPassword is the authenticated self-service rotation.
// Other sessions are revoked; this one continues via the re-issued
// cookies in the response.
func (s *Server) handleChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil || req.CurrentPassword == "" || req.NewPassword == "" {
		httpError(w, http.StatusBadRequest, "current_password + new_password required")
		return
	}
	res, err := s.authSvc.ChangePassword(r.Context(), id.UserID, req.CurrentPassword, req.NewPassword, r.UserAgent(), s.clientIP(r))
	if err != nil {
		httpError(w, mapAuthErrorStatus(err), "%s", err.Error())
		return
	}
	s.auditTenant(r, id.TeamID, "user.password_changed", "user", id.UserID, nil)
	s.renderAuthResponse(w, r, res)
}

// handleRevokeAllSessions is "sign out everywhere": every refresh
// session dies now; outstanding access JWTs age out within their
// ≤15-minute TTL. The caller's own cookies are cleared too.
func (s *Server) handleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.authSvc.RevokeUserSessions(r.Context(), id.UserID); err != nil {
		httpError(w, http.StatusInternalServerError, "%s", err.Error())
		return
	}
	s.auditTenant(r, id.TeamID, "user.sessions_revoked", "user", id.UserID, nil)
	s.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}
