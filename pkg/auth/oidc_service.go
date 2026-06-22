package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/auth/orgsso"
	"github.com/SocialGouv/iterion/pkg/identity"
)

// LoginWithExternal completes an OIDC/OAuth flow. It either:
//   - finds an existing user via OIDCLink → logs them in,
//   - finds a user by email → links the new identity to them,
//   - in SignupOpen mode, creates a fresh user (+ personal team),
//   - in SignupInviteOnly mode, returns ErrSignupClosed unless the
//     user was already provisioned.
func (s *Service) LoginWithExternal(ctx context.Context, ext oidc.ExternalUser, userAgent, ip string) (LoginResult, error) {
	if ext.Subject == "" {
		return LoginResult{}, fmt.Errorf("auth: external user missing subject")
	}
	if ext.Email == "" {
		return LoginResult{}, oidc.ErrEmailMissing
	}
	now := s.now().UTC()

	link, err := s.store.GetOIDCLink(ctx, ext.Provider, ext.Subject)
	if err == nil {
		u, err := s.store.GetUser(ctx, link.UserID)
		if err != nil {
			return LoginResult{}, err
		}
		if u.Status == identity.UserStatusDisabled {
			return LoginResult{}, ErrAccountDisabled
		}
		u.LastLoginAt = &now
		_ = s.store.UpdateUser(ctx, u)
		return s.issueLogin(ctx, u, userAgent, ip)
	}
	if !errors.Is(err, identity.ErrNotFound) {
		return LoginResult{}, err
	}

	email := identity.NormalizeEmail(ext.Email)
	u, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		if u.Status == identity.UserStatusDisabled {
			return LoginResult{}, ErrAccountDisabled
		}
		// Auto-link the new external identity onto the existing user
		// only when the operator has explicitly trusted this OIDC
		// provider's email verification. Otherwise return
		// ErrLinkRequiresConsent so the UI prompts the user to link
		// the connection from settings — the previous code
		// auto-linked unconditionally, which let any IdP that happened
		// to claim a registered user's email take over the account
		// (only mitigated by hoping every IdP rigorously verifies
		// email, which is not always the case for self-hosted /
		// emerging providers).
		if _, ok := s.trustedAutoLinkProviders[ext.Provider]; !ok {
			return LoginResult{}, ErrLinkRequiresConsent
		}
		if err := s.store.UpsertOIDCLink(ctx, identity.OIDCLink{
			Provider:       ext.Provider,
			ProviderUserID: ext.Subject,
			UserID:         u.ID,
			Email:          email,
			CreatedAt:      now,
		}); err != nil {
			return LoginResult{}, err
		}
		u.LastLoginAt = &now
		_ = s.store.UpdateUser(ctx, u)
		return s.issueLogin(ctx, u, userAgent, ip)
	}
	if !errors.Is(err, identity.ErrNotFound) {
		return LoginResult{}, err
	}

	// New user via SSO.
	if s.signupMode != SignupOpen {
		return LoginResult{}, ErrSignupClosed
	}
	u = identity.User{
		ID:        uuid.NewString(),
		Email:     email,
		Name:      ext.Name,
		Status:    identity.UserStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	u, err = s.store.CreateUser(ctx, u)
	if err != nil {
		return LoginResult{}, err
	}
	teamID, err := s.createPersonalTeam(ctx, u)
	if err != nil {
		return LoginResult{}, err
	}
	u.DefaultTeamID = teamID
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return LoginResult{}, err
	}
	if err := s.store.UpsertOIDCLink(ctx, identity.OIDCLink{
		Provider:       ext.Provider,
		ProviderUserID: ext.Subject,
		UserID:         u.ID,
		Email:          email,
		CreatedAt:      now,
	}); err != nil {
		return LoginResult{}, err
	}
	u.LastLoginAt = &now
	_ = s.store.UpdateUser(ctx, u)
	return s.issueLogin(ctx, u, userAgent, ip)
}

// LoginWithExternalForOrg completes a per-org OIDC flow (a tenant's own
// Keycloak, slug "oidc-org-<providerID>"). The org admin has explicitly
// designated this IdP as the org's trust root, so a NEW email is onboarded
// into the org regardless of the global SignupMode — but an SSO identity is
// NEVER auto-linked onto a pre-existing iterion account by email (that needs
// JWKS ID-token verification + explicit consent, a Phase-3 hardening; until
// then → ErrLinkRequiresConsent). The resolved user is granted membership in
// tenantID at the provider's DefaultRole (grant-only, never downgraded, capped
// below owner) and lands in that org.
func (s *Service) LoginWithExternalForOrg(ctx context.Context, ext oidc.ExternalUser, tenantID, providerID, userAgent, ip string) (LoginResult, error) {
	if s.orgSSO == nil {
		return LoginResult{}, oidc.ErrUnknownProvider
	}
	if ext.Subject == "" {
		return LoginResult{}, fmt.Errorf("auth: external user missing subject")
	}
	if ext.Email == "" {
		return LoginResult{}, oidc.ErrEmailMissing
	}
	row, err := s.orgSSO.Get(ctx, providerID)
	if err != nil {
		return LoginResult{}, oidc.ErrUnknownProvider
	}
	// Defence in depth: the provider must belong to the claimed tenant and be
	// an enabled OIDC row. The resolver already checked, but the login path
	// must not trust the slug→tenant mapping blindly.
	if row.TenantID != tenantID || row.Kind != orgsso.KindOIDC || !row.Enabled {
		return LoginResult{}, oidc.ErrUnknownProvider
	}
	role := row.DefaultRole
	if !role.Valid() || role == identity.RoleOwner {
		role = identity.RoleMember
	}
	now := s.now().UTC()

	// 1) Returning user — existing link for this org's provider slug.
	link, err := s.store.GetOIDCLink(ctx, ext.Provider, ext.Subject)
	if err == nil {
		u, err := s.store.GetUser(ctx, link.UserID)
		if err != nil {
			return LoginResult{}, err
		}
		if u.Status == identity.UserStatusDisabled {
			return LoginResult{}, ErrAccountDisabled
		}
		if err := s.grantMembership(ctx, u.ID, tenantID, role, now); err != nil {
			return LoginResult{}, err
		}
		u.LastLoginAt = &now
		_ = s.store.UpdateUser(ctx, u)
		return s.issueLoginInTeam(ctx, u, tenantID, userAgent, ip)
	}
	if !errors.Is(err, identity.ErrNotFound) {
		return LoginResult{}, err
	}

	// 2) Existing iterion account by email — never auto-link in V1.
	email := identity.NormalizeEmail(ext.Email)
	if _, err := s.store.GetUserByEmail(ctx, email); err == nil {
		return LoginResult{}, ErrLinkRequiresConsent
	} else if !errors.Is(err, identity.ErrNotFound) {
		return LoginResult{}, err
	}

	// 3) New user — onboard into the org (the admin opted this IdP in). No
	// personal team: the user belongs to the configuring org.
	u := identity.User{
		ID:            uuid.NewString(),
		Email:         email,
		Name:          ext.Name,
		Status:        identity.UserStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		DefaultTeamID: tenantID,
	}
	u, err = s.store.CreateUser(ctx, u)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.store.UpsertOIDCLink(ctx, identity.OIDCLink{
		Provider:       ext.Provider,
		ProviderUserID: ext.Subject,
		UserID:         u.ID,
		Email:          email,
		CreatedAt:      now,
	}); err != nil {
		return LoginResult{}, err
	}
	if err := s.grantMembership(ctx, u.ID, tenantID, role, now); err != nil {
		return LoginResult{}, err
	}
	u.LastLoginAt = &now
	_ = s.store.UpdateUser(ctx, u)
	return s.issueLoginInTeam(ctx, u, tenantID, userAgent, ip)
}

// grantMembership ensures userID has at least `role` in teamID. Grant-only: an
// existing membership at an equal-or-higher role is left untouched (never
// downgrade a manually-promoted user); a lower one is upgraded.
func (s *Service) grantMembership(ctx context.Context, userID, teamID string, role identity.Role, now time.Time) error {
	existing, err := s.store.GetMembership(ctx, userID, teamID)
	if errors.Is(err, identity.ErrNotFound) {
		return s.store.UpsertMembership(ctx, identity.Membership{UserID: userID, TeamID: teamID, Role: role, JoinedAt: now})
	}
	if err != nil {
		return err
	}
	if existing.Role.AtLeast(role) {
		return nil
	}
	existing.Role = role
	return s.store.UpsertMembership(ctx, existing)
}

// SwitchTeamWithCookie is identical to SwitchTeam but also returns
// the AccessTTL so the caller can stamp the cookie max-age in
// lock-step with the JWT.
func (s *Service) SwitchTeamWithCookie(ctx context.Context, userID, teamID string) (Identity, string, time.Time, error) {
	return s.SwitchTeam(ctx, userID, teamID)
}
