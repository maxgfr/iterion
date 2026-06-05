---
name: finding-taxonomy
description: |
  Canonical enum of finding types for sec-audit-source. The triage
  and revalidate nodes MUST classify every candidate into exactly
  one of these categories. Load this skill in any node that emits
  or normalises findings.
---

# Finding taxonomy

Twelve categories. Every candidate emitted by `triage` MUST carry
exactly one `finding_type` from this list. Mapping novel scanner
rule IDs to one of these is the triage node's responsibility — when
unsure, pick `other` rather than inventing a new category.

## Categories

| `finding_type` | What it captures | Typical scanner signals |
|---|---|---|
| `injection` | SQL / NoSQL / LDAP / command / template / GraphQL injection | semgrep `*-injection`, gosec G201/G204, bandit B608/B609 |
| `xss` | Reflected, stored, DOM-based cross-site scripting | semgrep `*-xss`, ESLint react/no-danger |
| `ssrf` | Server-side request forgery, blind SSRF, gopher-style SSRF | semgrep `ssrf`, custom matchers on outbound calls with user-controlled URLs |
| `auth` | Missing/weak authentication, broken JWT/session, MFA bypass | semgrep `*-jwt-*`, custom `route-no-auth` matchers |
| `authz` / `idor` | Missing authorisation, insecure direct object reference, tenant-crossing | custom `*-no-rls`, `*-missing-companyId-filter` matchers |
| `crypto` | Weak ciphers, predictable IVs, hardcoded keys, missing AEAD | semgrep `*-weak-crypto`, gosec G401/G402, bandit B324 |
| `secrets` | Hardcoded credentials, API keys in source, AWS/GCP/Stripe tokens | gitleaks, semgrep `*-hardcoded-secret`, trufflehog |
| `deserialization` | Unsafe `pickle`, `yaml.load`, `eval(json)`, native deserialization | semgrep `*-unsafe-deserialization`, bandit B301-B307 |
| `path-trav` | Path traversal, unsanitized file paths, archive extraction | semgrep `*-path-traversal`, semgrep `tarfile-extractall` |
| `redirect` | Open redirect, untrusted URL in `Location` header, OAuth `redirect_uri` abuse | semgrep `unsafe-redirect`, custom matchers |
| `config` | Misconfigured framework or runtime (Django DEBUG=True, CORS *, secure flag off) | trivy fs config, semgrep `*-debug-true`, gosec G402 TLS |
| `other` | Real signal that doesn't fit above (e.g. log injection, race in auth flow) | fallback only |

## Severity heuristics

Severity is set by `triage` per finding. Default mapping from scanner
output:

| Scanner severity | Default `severity` | Bump rules |
|---|---|---|
| critical / error | `critical` | Demote to `high` if no concrete exploit path. |
| high / warning | `high` | Bump to `critical` if internet-reachable + auth-bypass. |
| medium / info | `medium` | Bump to `high` if `secrets` in production config. |
| low | `low` | Bump to `medium` if `authz`/`idor` with cross-tenant scope. |

Auth bypass and IDOR are bumped one level when:
- the affected endpoint is exposed publicly (no auth middleware
  upstream), AND
- the finding lets a request access another tenant's resource.

## Label encoding (board)

When the `report_card` node creates a kanban issue, labels are
emitted as:

- `severity:<level>` — `low` | `medium` | `high` | `critical`
- `type:<finding-type>` — e.g. `type:ssrf`, `type:authz`
- `scanner:<id>` — primary scanner that flagged it (`semgrep`,
  `gosec`, `bandit`, `gitleaks`, `trivy`)
- `source:sec-audit-source` — always; lets a follow-up bot filter
  to security findings only
- `triage-uncertain` — when the judge couldn't decide; flags for
  human review

## Mapping examples

| Raw scanner output | `finding_type` | `severity` |
|---|---|---|
| `gosec G201: SQL string formatting` | `injection` | `high` |
| `gitleaks: AWS Access Key found in config/prod.yaml` | `secrets` | `critical` |
| `semgrep js.express.security.audit.express-jwt-not-revoked` | `auth` | `medium` |
| `bandit B301: pickle.loads() of untrusted data` | `deserialization` | `high` |
| `trivy: Container as root` | `config` | `medium` |
| `semgrep python.django.security.audit.unrestricted-file-upload` | `path-trav` | `high` |

When in doubt, pick the more specific category over the generic one
(prefer `idor` over `authz` if cross-tenant scope is established;
prefer `secrets` over `crypto` if the issue is a hardcoded key).
