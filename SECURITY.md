# Security Policy

## Supported versions

iterion ships from a single mainline. The latest released minor version
receives security fixes; older minors are only patched when an
operational impact is documented and reproducible.

| Version | Supported          |
| ------- | ------------------ |
| Latest minor (see [releases](https://github.com/SocialGouv/iterion/releases)) | Yes |
| Previous minor | Critical fixes only, best-effort |
| Older         | No |

## Reporting a vulnerability

Please use GitHub's private **Security Advisory** workflow:

1. Go to the repository's **Security** tab → *Report a vulnerability*.
2. File a private advisory describing the impact, the affected version,
   and a minimal reproducer if you have one.

This routes the report to the maintainers without going through public
issues. We aim to acknowledge within **3 business days** and, for
confirmed issues, ship a patch within **30 days** for high-severity
findings and **90 days** for medium-severity.

We will credit reporters in the released advisory unless you ask
otherwise.

## Threat model & scope

iterion is a workflow engine that drives LLM backends (Anthropic,
OpenAI, …) and executes shell tools the operator authorises. The
in-scope surface for vulnerability reports:

- **Auth / identity** (`pkg/auth/`, `pkg/identity/`, `pkg/secrets/`):
  authentication bypass, privilege escalation, tenant isolation
  failures, secret exfiltration.
- **Server HTTP / WS** (`pkg/server/`): unauthenticated access to
  authenticated endpoints, cross-tenant data leaks, CSRF on
  state-changing routes, injection via untrusted headers/queries.
- **Sandbox** (`pkg/sandbox/`): container escape, allowlist bypass on
  the network proxy, mount-traversal from inside the sandbox.
- **Vendor supply chain** (`vendor/`): malicious dependency, broken
  pinning, untrusted code path.
- **Bundle handling** (`pkg/bundle/`): tar/zip extraction issues,
  symlink-traversal, path-traversal.
- **Cloud deployment** (`charts/`, `pkg/cloud/`, `Dockerfile`):
  privileged container, exposed admin endpoint, secret in
  values.yaml example.

The following are **out of scope** unless they amount to one of the
above:

- DSL workflows that authorise dangerous tools when the operator
  explicitly configures them — that is by design.
- Theoretical denial-of-service requiring an authenticated operator
  account.
- Misconfiguration in the operator's deployment (broad NetworkPolicy,
  weak passwords, leaked credentials) that does not stem from a
  defective default in this repo.
- Issues only reachable when iterion is run on a custom fork.

## Coordinated disclosure

We ask for a 90-day non-disclosure window starting from acknowledgement
for high-severity findings, shortened by mutual agreement once a fix is
released. Pre-disclosure to downstream operators (managed deployments,
notable adopters) happens on a need-to-know basis the maintainers
coordinate.

## Hardening guidance

The [`docs/cloud-public-exposure-checklist.md`](docs/cloud-public-exposure-checklist.md)
captures the operator-facing hardening steps that complement this
policy (TLS, NetworkPolicies, secret rotation, audit logging).
