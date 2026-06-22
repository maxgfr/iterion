# ADR-034: Webhook auth supports token headers and body HMAC

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/webhooks/types.go](../../pkg/webhooks/types.go), [pkg/server/middleware_webhook.go](../../pkg/server/middleware_webhook.go)

## Context

Forge webhook providers authenticate deliveries differently. GitLab-style webhooks can present a plaintext shared token in a header. GitHub and Forgejo authenticate the raw request body with an HMAC signature and do not echo the token in a header.

Middleware wants one public webhook entry path, but HMAC verification must happen over the exact raw body bytes before any side effect. Reading or rejecting the body too early in shared middleware would break provider-specific verification.

## Decision

Webhook configuration carries a unified `SignatureMode` in [`pkg/webhooks/types.go`](../../pkg/webhooks/types.go). The default token mode uses a header-presented token. HMAC mode uses a hex HMAC-SHA256 signature over the raw body and stores the plaintext secret sealed at rest for recomputation.

The webhook middleware in [`pkg/server/middleware_webhook.go`](../../pkg/server/middleware_webhook.go) checks header tokens only when `SignMode` is not HMAC. For HMAC providers, it resolves config and performs admission scaffolding without consuming the request body, leaving the provider handler responsible for verifying the HMAC over raw bytes before any side effect.

This keeps one middleware path for config lookup, rate/quota/suspend checks, and tenant identity stamping while preserving the provider-specific authentication primitive.

## Trade-offs

| Dimension | Dual `SignatureMode` | HMAC-only | Separate endpoints per auth scheme |
|---|---|---|
| Provider fit | Matches GitLab token headers and GitHub/Forgejo body signatures. | Forces token-header providers into an unnatural scheme. | Matches providers but duplicates routing/middleware. |
| Raw-body safety | Middleware skips body reads for HMAC mode. | Body handling can be centralised. | Each endpoint must preserve raw-body rules. |
| API shape | One enum and one config model. | Simpler enum-free model. | More endpoint/config surface. |

The honest concession is that authentication is split between middleware and provider handlers for HMAC mode.

## Alternatives considered

### 1. Require HMAC for every provider

Iterion could have normalized all inbound webhooks to raw-body HMAC verification.

**Rejected because**: GitLab's native model is a plaintext token header, and forcing HMAC would not match provider behaviour or operator setup.

### 2. Use separate endpoints for token and HMAC webhooks

Each auth mode could have had a distinct route and middleware stack.

**Rejected because**: config lookup, quota, suspend checks, and tenant identity stamping would be duplicated while provider identity is already part of the webhook config.

## Consequences

- **Provider-native authentication is preserved.** GitLab can use token headers while GitHub/Forgejo can use body signatures.
- **HMAC verification keeps raw bytes intact.** Shared middleware does not consume the body for HMAC-mode configs.
- **Handlers carry a security obligation.** HMAC-mode provider handlers must verify the signature before any side effect.
- **The enum is the extension point.** Future auth schemes can add a mode without multiplying endpoint families.
- **Rechallenge if providers require new proof types.** mTLS or another provider-specific scheme should be added as a new auth mode rather than forced into token or HMAC.
