package server

import (
	"context"

	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/auth/orgsso"
	"github.com/SocialGouv/iterion/pkg/secure/httpdial"
)

// ssoStrict reports whether outbound OIDC fetches (per-org issuer discovery,
// token, userinfo) must enforce the public-unicast SSRF guard: always in cloud
// mode, and in local mode when bound to a non-loopback address (multi-user dev
// box / LAN exposure). Mirrors the preview-proxy strictness derivation.
func (s *Server) ssoStrict() bool {
	return s.cfg.Mode == "cloud" || !httpdial.IsLoopbackBind(s.cfg.Bind)
}

// resolveConnector returns the OIDC connector for a provider slug. Global
// connectors (github, google, the deployment-wide "sso") come from the static
// registry; a per-org slug ("oidc-org-<id>") is built on demand from the
// tenant's stored provider row, with an SSRF-guarded HTTP client.
//
// tenantID and providerID are non-empty only for per-org connectors. The
// callback drives the per-org login from the values it persisted in
// PendingAuth at /start — never from these resolver outputs or the URL — so a
// slug presented at /callback can never be coerced into another tenant's
// policy (provider/tenant confusion is closed).
func (s *Server) resolveConnector(ctx context.Context, slug string) (oidc.Connector, string, string, error) {
	if s.oidcRegistry != nil {
		if c, err := s.oidcRegistry.Get(slug); err == nil {
			return c, "", "", nil
		}
	}
	id, ok := orgsso.ParseOIDCSlug(slug)
	if !ok || s.orgSSO == nil || s.sealer == nil {
		return nil, "", "", oidc.ErrUnknownProvider
	}
	row, err := s.orgSSO.Get(ctx, id)
	if err != nil || row.Kind != orgsso.KindOIDC || !row.Enabled {
		return nil, "", "", oidc.ErrUnknownProvider
	}
	secret, err := orgsso.OpenClientSecret(s.sealer, row.ID, row.SealedSecret)
	if err != nil {
		return nil, "", "", oidc.ErrUnknownProvider
	}
	strict := s.ssoStrict()
	conn := oidc.NewGenericConnectorWithSlug(
		slug, row.IssuerURL, row.ClientID, secret, row.DisplayName, row.Scopes,
		oidc.SafeGenericClient(strict), strict,
	)
	return conn, row.TenantID, row.ID, nil
}
