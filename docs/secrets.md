# Secrets protection

Iterion runs agents that can be prompt-injected (the sec-audit bots read
untrusted repo content), shell out, and read files. Two distinct leak
surfaces are defended in layers:

1. **Exfiltration** — the agent sends a secret off-box.
2. **Observability leak** — a secret lands in clear in `events.jsonl`,
   artifacts, `run.log`, the studio/board stream, or `report.md`.

The engine is [`pkg/backend/secretguard`](../pkg/backend/secretguard):
a per-run `Guard` built by
[`model.BuildSecretGuard`](../pkg/backend/model/secretguard.go) from the
run's resolved credentials, sensitive host env vars, and the workflow's
declared `secrets:` block.

## Detection (incl. base64 and other encodings)

Two tiers:

- **Known-value taint (deterministic).** Iterion knows its secret
  values, so for each it precomputes every textual form — raw, base64
  (std + url, ±padding), hex (upper/lower), URL-escape, JSON-escape —
  and matches those literally via a single RE2 alternation. This is the
  reliable answer to "also detect base64": we match the base64 form of a
  secret we *hold*, we don't guess. Zero encoding false-negatives.
- **Heuristic (for unknown secrets).** The gitleaks-derived detector
  ([`tool/privacy/detector`](../pkg/backend/tool/privacy/detector)) +
  Shannon entropy, plus a recursive base64/hex decode pass that peels one
  layer off a blob and re-scans (catches an AKIA/JWT wrapped in base64
  that the agent read from a file iterion never registered).

## Layer 0 — sink redaction (default on)

`Guard.Redact` scrubs known values (any encoding) → their placeholder,
and unknown token shapes → `[redacted]`, at every **observational**
sink, before persistence:

- events.jsonl (all event types, via a redacting `AppendEvent` wrapper +
  `node_finished` output via the engine's `SecretScrubber`),
- run.log block bodies, tool sidecar blobs, turn-snapshot conversations.

**Deliberately NOT redacted:** persisted **artifacts** and the resume
**checkpoint**. These are load-bearing — they feed `{{outputs.X}}` /
`{{artifacts.X}}` and are re-read on resume; redacting them would corrupt
cross-node and cross-resume data flow. Their defence is Layer 1
(placeholders keep the real secret out of node output in the first place)
plus the run store being local/private.

## Layer 1 — placeholders + materialization (default on)

Declare secrets in the DSL; the agent only ever sees an opaque
placeholder `__ITERION_SECRET_<name>__`; iterion swaps in the real value
at the moment of execution.

```iter
secrets:
  github_token: "${GITHUB_TOKEN}"          # short form
  deploy_key:
    value: "${DEPLOY_KEY}"
    hosts: ["api.github.com", "github.com"] # egress scoping (Layer 2)
```

Reference as `{{secrets.deploy_key}}` in prompts and tool/shell commands.
Materialization happens immediately before exec, keeping the placeholder
form in every hook/log:

- **claw** `tool` nodes (shell + script) and the in-process tool loop
  (`executeToolsDirect`) call `Guard.Materialize` before exec.
- **claude_code** uses a `PreToolUse` hook returning `UpdatedInput` with
  the materialised tool input (the SDK-supported substitution path).
- Both via `delegate.Task.MaterializeSecrets` (a closure, set by the
  executor), so `pkg/backend/delegate` stays decoupled from secretguard.

Plus a **behavioural backstop**: a "## Secret handling" system-prompt
clause (both backends) tells the agent not to read/exfiltrate credential
files and to pass placeholders through verbatim. Never the primary
control — the structural boundary is the materialization above.

## Layer 2 — TLS-inspection egress (default on for sandboxed runs)

For secrets the agent uses in its *own* TLS calls (e.g. claude_code's
Bash `curl`/`git push`), the sandbox egress proxy
([`pkg/sandbox/netproxy`](../pkg/sandbox/netproxy)) can terminate TLS and
rewrite the plaintext request (Deno-parity secret handling):

- A per-run **ephemeral CA** ([`ca.go`](../pkg/sandbox/netproxy/ca.go),
  in-memory, never persisted) mints per-host leaves. Its public cert is
  injected into the sandbox so in-container clients trust the leaves.
- **Substitution** ([`inspect.go`](../pkg/sandbox/netproxy/inspect.go)):
  `MaterializeForHost` swaps placeholder→value, but only toward a
  secret's approved `hosts:`.
- **Content DLP**: `ExfiltratesTo` blocks (403) a real secret value bound
  for a host it isn't scoped to — defeats domain-fronting the host
  allowlist can't see.

Inspection activates by default when a sandboxed run has known secrets;
it forces a proxy even under `network: open`. Why TLS inspection is safe
to do: Claude Code and the Anthropic/OpenAI SDKs are standard trust-store
clients with **no certificate pinning** (per the [official Claude Code
network-config docs](https://code.claude.com/docs/en/network-config) —
they work behind Zscaler/CrowdStrike/mitmproxy once the CA is trusted).
The default-transparent proxy is a cost choice, not a pinning constraint.

### Limitation: OAuth-forfait credentials are NOT substituted

For **OAuth-forfait** auth (Anthropic Claude Code OAuth, OpenAI
ChatGPT/Codex — the recommended credential model), egress substitution is
impractical: the CLI performs stateful token refresh, and the Consumer
Terms scope the forfait to Claude Code only (no API key to splice). Those
credentials are protected by **the network allowlist + Layer 0 redaction
+ the backstop clause**, not by Layer 2 substitution. Layer 2's
substitution/DLP value is for **declared workflow secrets** (a
`GITHUB_TOKEN`, a deploy key) and API-key mode.

### Status: live-validated in a docker sandbox (with trust-store caveat)

The MITM mechanism is hermetically tested end-to-end
([`inspect_test.go`](../pkg/sandbox/netproxy/inspect_test.go)) **and**
live-validated in a real docker sandbox (2026-06-08): a sandboxed `tool`
run with a `secrets:` entry scoped `hosts: ["example.com"]` confirmed
`inspect=true`, the per-run CA bind-mounted at `/run/iterion/egress-ca.pem`
and trusted in-container, a `--data {{secrets.X}}` call to the approved
host forwarded through the MITM to the real upstream (HTTP 405 from
example.com), and the same call to an unapproved host blocked by content
DLP (HTTP 403 + `secret exfiltration blocked` event). The real value
never appeared in the run store.

Remaining follow-ups before relying on it broadly:

- **Trust-store breadth.** Only `NODE_EXTRA_CA_CERTS` (additive, Node /
  Claude Code — the dominant LLM path) is injected. Non-Node clients
  (curl, python via `certifi`, git) need the CA in the OS store
  (`update-ca-certificates`) — in the live test curl had to be pointed at
  the CA with `--cacert`. The slim image runs as non-root with no
  `update-ca-certificates`, so OS-store injection wants a root post-create
  step (or `SSL_CERT_FILE`/`CURL_CA_BUNDLE`/`REQUESTS_CA_BUNDLE`).
- **undici / WebFetch gotcha.** Claude Code's `WebFetch` tool bundles its
  own undici dispatcher that can ignore `NODE_EXTRA_CA_CERTS` — validate
  it picks up the CA, or document `skipWebFetchPreflight`.
- **Kubernetes driver.** Inspection is auto-disabled where
  `Capabilities.SupportsTLSInspection` is false (k8s, noop), degrading to
  Layer 1 + redaction + allowlist; k8s CA injection (ConfigMap/Secret) is
  a follow-up.

## Environment kill-switches

| Var | Default | Effect |
|---|---|---|
| `ITERION_SECRETS_REDACT` | on | Master: off disables Layer 0 sink redaction (materialization still works). |
| `ITERION_SECRETS_REDACT_HEURISTIC` | on | off keeps known-value redaction but disables the gitleaks/entropy pass. |
| `ITERION_SECRETS_REDACT_DECODE` | on | off disables the recursive base64/hex decode pass. |
| `ITERION_SECRETS_REDACT_MIN_SCORE` | 0.7 | Heuristic confidence floor (the 0.6 generic high-entropy rule is excluded by default). |
| `ITERION_SECRETS_PLACEHOLDERS` | on | off renders `{{secrets.X}}` as the real value instead of a placeholder. |
| `ITERION_SANDBOX_TLS_INSPECT` | on | off disables Layer 2 TLS inspection (the escape hatch for a pinning client or broken CA injection). |

## Diagnostics

`C090` duplicate secret · `C091` secret/var name collision · `C092`
malformed egress host (Layer 2) · `C093` `{{secrets.X}}` references an
undeclared secret.
