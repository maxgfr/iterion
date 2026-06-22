# ADR-039: Per-tenant sealed OAuth app store replaces global env

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/forge/oauth_app_store.go](../../pkg/forge/oauth_app_store.go), [pkg/forge/oauth_app_sealer.go](../../pkg/forge/oauth_app_sealer.go)

## Context

Multi-tenant cloud deployments must support many forge providers and instances. A single process-global set of OAuth client credentials cannot represent multiple tenants, self-hosted forge base URLs, or distinct credential ownership boundaries.

Client secrets also need at-rest isolation: a secret for one tenant's forge OAuth application must not be silently transplanted onto another app record or serialized back through API responses.

## Decision

Forge OAuth application credentials are stored per tenant, provider, and forge base URL in [`pkg/forge/oauth_app_store.go`](../../pkg/forge/oauth_app_store.go). The Mongo schema enforces a unique `(tenant_id, provider, forge_base_url)` index, and the in-memory store mirrors the same uniqueness rule.

`ForgeOAuthApp` stores `ClientID` in the clear because it is not secret and the admin UI lists it. It stores the client secret only as `SealedSecret`, omitted from JSON serialization.

Secrets are sealed and opened by [`pkg/forge/oauth_app_sealer.go`](../../pkg/forge/oauth_app_sealer.go) with associated data `forge_oauth_app:<id>`. That AAD binds the encrypted secret to the specific app record and prevents cross-record transplant from being accepted silently.

## Trade-offs

| Dimension | Per-tenant sealed app store | Global environment variables |
|---|---|---|
| Tenant cardinality | Supports many tenants/providers/instances. | One or few process-wide credentials. |
| Credential isolation | Secrets are per row and AAD-bound to app id. | Credentials are shared by process configuration. |
| Operational simplicity | Requires CRUD/storage/sealing flows. | Simple deployment-time env config. |
| Self-hosted forges | Base URL is part of identity. | Env var naming does not scale cleanly by instance. |

The honest concession is that per-tenant OAuth app management adds operational UI/API and storage complexity.

## Alternatives considered

### 1. Keep global `ITERION_FORGE_*_OAUTH_*` environment variables

The connect flow could have continued resolving OAuth credentials from process environment.

**Rejected because**: global env vars do not scale to multiple tenants, multiple providers, and multiple self-hosted forge instances with isolated credentials.

### 2. Store one shared operator-managed app per provider

Cloud could have used a shared OAuth app for all tenants of a provider.

**Rejected because**: tenants may connect distinct self-hosted forge instances and need credential ownership/isolation that a shared app cannot provide.

## Consequences

- **Forge OAuth credentials are tenant-scoped.** Each tenant/provider/base URL tuple can resolve to its own app credentials.
- **Secrets are never serialized.** API responses can include client IDs and metadata without exposing client secrets.
- **Sealed blobs are record-bound.** AAD `forge_oauth_app:<id>` prevents silent secret swapping across app rows.
- **The legacy global-env model is no longer the scaling path.** Operators should treat env credentials as insufficient for multi-tenant cloud cardinality.
- **Rechallenge if auto-provisioning fails operationally.** If tenant-managed app provisioning proves unreliable, a shared operator-managed model may need reconsideration.
