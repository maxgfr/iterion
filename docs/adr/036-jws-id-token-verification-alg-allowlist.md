# ADR-036: JWS ID-token verification uses an asymmetric algorithm allowlist

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/auth/oidc/jwks.go](../../pkg/auth/oidc/jwks.go)

## Context

Per-org OIDC lets tenant administrators configure arbitrary issuers. That means ID-token verification consumes keys and tokens from many administrative domains, and the JWT algorithm header cannot be treated as trustworthy policy.

Classic JWS algorithm-confusion attacks abuse libraries that accept `none` or HMAC algorithms when the verifier expected an asymmetric signature. With JWKS public keys, accepting HS256 can turn the public key into an HMAC secret.

## Decision

ID-token verification in [`pkg/auth/oidc/jwks.go`](../../pkg/auth/oidc/jwks.go) uses a fixed `jwksVerifyAlgs` allowlist and passes it to `jwt.WithValidMethods` during `ParseWithClaims`.

The allowlist contains asymmetric RSA, RSA-PSS, and ECDSA algorithms at 256/384/512 strengths: `RS256`, `RS384`, `RS512`, `PS256`, `PS384`, `PS512`, `ES256`, `ES384`, and `ES512`. It deliberately excludes `none` and the HMAC family.

This makes the configured issuer's JWKS key material usable only for asymmetric signature verification, regardless of what `alg` value appears in the token header.

## Trade-offs

| Dimension | Asymmetric allowlist | Accept all library-supported algorithms |
|---|---|---|
| Algorithm-confusion resistance | Excludes `none` and HMAC-with-public-key attacks. | Depends on token header/library defaults. |
| Interop breadth | Supports common OIDC asymmetric algorithms. | Supports more algorithms automatically. |
| Future algorithms | Requires explicit code change for new safe algorithms. | New library algorithms are accepted implicitly. |

The honest concession is that new standardized asymmetric algorithms require an intentional allowlist update.

## Alternatives considered

### 1. Trust the JWT library's supported algorithms

The verifier could have omitted `jwt.WithValidMethods` and let the library accept any algorithm it supports.

**Rejected because**: arbitrary per-org issuers create an algorithm-confusion risk, especially for `none` and HMAC algorithms with JWKS public keys.

### 2. Allow only RSA algorithms

The verifier could have restricted tokens to RS256/384/512.

**Rejected because**: standards-compliant OIDC providers may use RSA-PSS or ECDSA, and those are asymmetric algorithms that satisfy the security constraint.

## Consequences

- **`alg=none` is not accepted.** Unsigned tokens cannot pass ID-token verification.
- **HMAC confusion is blocked.** JWKS public keys are not reused as symmetric HMAC secrets.
- **OIDC interop remains broad.** RSA, RSA-PSS, and ECDSA providers are supported.
- **Algorithm additions are deliberate.** Operators get a clear code review point for new JWT algorithms.
- **Rechallenge for EdDSA or similar.** If a tenant requires a standardized asymmetric algorithm such as EdDSA, it should be added explicitly.
