# ADR-035: Per-org SSO uses generic OIDC plus dedicated connectors

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/auth/oidc/generic.go](../../pkg/auth/oidc/generic.go), [pkg/auth/oidc/github.go](../../pkg/auth/oidc/github.go), [pkg/auth/oidc/google.go](../../pkg/auth/oidc/google.go)

## Context

Per-org SSO must support arbitrary standards-compliant OIDC identity providers, including org-admin-supplied issuers. It also needs to support providers with important quirks that do not fit the same discovery and userinfo assumptions.

GitHub's login integration is OAuth2 rather than full OIDC for the fields iterion needs: verified primary email comes from a separate emails endpoint, and team-gating needs GitHub org/team API calls. Google is a standard provider, but its endpoints and `email_verified` semantics are important enough to keep as a dedicated, stable connector.

## Decision

Iterion uses a discovery-based generic OIDC connector in [`pkg/auth/oidc/generic.go`](../../pkg/auth/oidc/generic.go) for standard IdPs. It fetches `/.well-known/openid-configuration`, validates discovered endpoints, caches discovery for a bounded TTL, and can be constructed with an SSRF-guarded HTTP client for per-org issuers.

GitHub has a dedicated connector in [`pkg/auth/oidc/github.go`](../../pkg/auth/oidc/github.go). It uses GitHub OAuth endpoints, fetches the user profile, resolves the verified primary email through the separate emails endpoint, and requests org/team scopes for team-gated login.

Google has a dedicated connector in [`pkg/auth/oidc/google.go`](../../pkg/auth/oidc/google.go). It keeps Google's well-known endpoints and email verification handling explicit rather than relying on arbitrary discovery at runtime.

## Trade-offs

| Dimension | Generic OIDC plus dedicated connectors | Single generic connector |
|---|---|---|
| Arbitrary IdPs | Discovery handles standard per-org issuers. | Discovery handles standard issuers. |
| GitHub support | Dedicated OAuth/API flow handles email and teams. | Generic OIDC cannot obtain GitHub's verified primary email/team data. |
| Google stability | Hardcoded connector avoids endpoint drift surprises. | Discovery is more uniform but less explicit. |
| Maintenance | More connector code paths. | One connector path. |

The honest concession is that provider-specific code paths must be tested and kept semantically aligned.

## Alternatives considered

### 1. Use one generic OIDC connector for all providers

Every provider could have been forced through discovery, token exchange, and userinfo parsing.

**Rejected because**: GitHub lacks the standard userinfo `email_verified` shape iterion needs and requires a separate email endpoint, plus team membership APIs for gating.

### 2. Use only dedicated connectors

Iterion could have required a bespoke connector for each supported IdP.

**Rejected because**: per-org SSO must support arbitrary OIDC providers such as Keycloak/Auth0/Azure-compatible issuers without code changes for every tenant.

## Consequences

- **Standard per-org IdPs are self-serve.** Generic discovery supports arbitrary compliant issuers.
- **Provider quirks stay explicit.** GitHub and Google semantics are not hidden behind a leaky generic abstraction.
- **SSRF hardening remains part of generic resolution.** Org-admin-supplied issuers use the safe client path.
- **Connector count grows with non-standard providers.** New providers with non-standard semantics should get dedicated connectors.
- **Rechallenge if GitHub changes.** If GitHub provides full OIDC userinfo with usable `email_verified` and team semantics, the dedicated connector can be reconsidered.
