# BYOK API keys (cloud) — per-org, per-user, per-webhook

How iterion-cloud resolves the LLM provider API keys a run uses. The
short version: **keys are owned by the org, sealed at rest in Mongo, and
resolved per-run with a precedence chain** (per-webhook override → user
default → org default → deployment env fallback). Nothing here is a
global plaintext secret the agent can read.

This document exists so we don't reverse-engineer the resolver again.
Every claim is anchored to a file:line; if the code moved, fix the
anchor in the same change.

## Mental model

```
              ┌─ per-webhook override  (Config.KeyOverrides[provider] → key_id)   ← highest
 a run's      ├─ requesting user's default   (ScopeUserID==me, IsDefault)
 provider  ───┤  requesting user's other key (ScopeUserID==me)
 key is       ├─ org default                 (ScopeUserID=="" , IsDefault)
 the first    ├─ org other key               (ScopeUserID=="")
 match in:    └─ deployment env fallback      (ANTHROPIC_API_KEY/… on the pod) ← lowest
```

A run launched by a **webhook** has the synthetic owner `webhook:<id>`
(no real user), so the user-scoped tiers are empty for it — its chain
collapses to **per-webhook override → org default → env fallback**.
That is exactly the "default per org, overridable per webhook" model.

## Storage — the `api_keys` Mongo collection

One document per key, sealed at rest. [pkg/secrets/byok.go](../pkg/secrets/byok.go):

| field | meaning |
|---|---|
| `_id` | key id (`secrets.NewApiKeyID()`) — what a webhook override references |
| `tenant_id` | owning org; every store call is tenant-filtered (fail-closed) |
| `scope_team` | the team the key belongs to |
| `scope_user` | set ⇒ user-scoped (personal); empty ⇒ **org-wide** |
| `provider` | `anthropic` \| `openai` \| `bedrock` \| `vertex` \| `azure` \| `openrouter` \| `xai` \| `zai` ([byok.go:50-63](../pkg/secrets/byok.go#L50)) |
| `name` | human label |
| `last4` / `fingerprint` | shown in UI; the key itself is never returned |
| `sealed_secret` | the ciphertext (`SealAPIKey(sealer, keyID, plaintext)`); JSON-hidden (`json:"-"`) |
| `is_default` | the default for its `(team, user, provider)` tuple — `ClearDefault` keeps it unique |
| `last_used_at` | best-effort observability (`MarkUsed`, fired detached off the launch path) |
| `expires_at` | optional |

- Interface: `ApiKeyStore` (Create/Get/Update/Delete/ListByTeam/ListByUser/MarkUsed/ClearDefault) — [byok.go:112](../pkg/secrets/byok.go#L112).
- Backings: `MongoApiKeyStore` (prod) + `MemoryApiKeyStore` (tests).
- Wired in the server at [cmd/iterion/server.go:193](../cmd/iterion/server.go#L193) (`NewMongoApiKeyStore(st.DB())` + `EnsureSchema`), handed to both the HTTP server (`ApiKeys:` config) and the cloud publisher.

The plaintext is sealed with the server's `Sealer` before it touches
Mongo, and is only unsealed transiently inside `resolveAndSealCredentials`
to be re-sealed into the per-run bundle. It is never written to logs,
events, artifacts, or returned by the API.

## Resolution — `secrets.Resolve`

[pkg/secrets/byok.go:168](../pkg/secrets/byok.go#L168):

```go
Resolve(ctx, store, teamID, userID string,
        providers []Provider,
        keyOverrides map[Provider]string,   // provider → key_id
        sealer) (map[Provider]Resolution, error)
```

Two passes over the keys visible from `(teamID, userID)`:

1. **Pass 1 — explicit overrides.** For each `provider → key_id` in
   `keyOverrides`, pin that exact key (must be visible + the right
   provider). This is the per-webhook override hook.
2. **Pass 2 — priority walk.** For any provider not already pinned, take
   the first key in `keyRank` order ([byok.go:234](../pkg/secrets/byok.go#L234)):

   | rank | key |
   |---|---|
   | 0 | requesting user's **default** (`scope_user==me && is_default`) |
   | 1 | requesting user's other key |
   | 2 | org **default** (`scope_user=="" && is_default`) |
   | 3 | org other key |
   | 99 | another user's personal key — **never applies** |

The publisher calls it for `allKnownProviders` ([publisher.go:138](../pkg/server/cloudpublisher/publisher.go#L138)) and seals whatever resolved into the run bundle.

## Where the publisher uses it

[pkg/server/cloudpublisher/publisher.go:167](../pkg/server/cloudpublisher/publisher.go#L167)
`resolveAndSealCredentials`, step 1 ("BYOK API keys",
[L189](../pkg/server/cloudpublisher/publisher.go#L189)):

```go
resolved, _ := secrets.Resolve(ctx, p.apiKeys, tenantID, ownerID,
                               allKnownProviders, nil /* keyOverrides */, p.sealer)
for prov, r := range resolved { bundle.APIKeys[prov] = string(r.Plaintext) }
```

The bundle is sealed under a fresh `secrets_ref`; the runner unseals it
and stamps `bundle.APIKeys` into ctx ([pkg/secrets/credentials.go](../pkg/secrets/credentials.go)).
The claude_code / claw delegates read the key from ctx.

> **Env fallback.** When the bundle has *no* key for a provider, the
> resolved bundle is empty for it and the runner falls back to the pod
> env (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …). That is the *only*
> role of the deployment-level `iterion-llm` Secret — a fallback for
> orgs that haven't entered their own keys, **not** the primary path.

## REST API

[pkg/server/byok_routes.go](../pkg/server/byok_routes.go). All under `requireAuth`; key values are write-only (never returned).

| verb + path | role |
|---|---|
| `GET /api/teams/{id}/api-keys` | list org + my keys visible from the team |
| `POST /api/teams/{id}/api-keys` | create an **org-wide** key |
| `GET/POST /api/me/api-keys` | list / create a **personal** key |
| `PATCH /api/teams/{id}/api-keys/{key_id}` | rename / promote to default |
| `DELETE /api/teams/{id}/api-keys/{key_id}` | revoke |

Create body ([byok_routes.go:132](../pkg/server/byok_routes.go#L132)): `{ "provider": "anthropic", "name": "...", "secret": "<key>", "is_default": true }`. The server seals `secret` and stores only the ciphertext + `last4`.

Studio UI: Settings → API Keys ([studio/src/views/Settings/ApiKeysTab.tsx](../studio/src/views/Settings/ApiKeysTab.tsx), [studio/src/api/byok.ts](../studio/src/api/byok.ts)).

## Per-webhook key override

**Goal:** a webhook can pin a *specific* key per provider, overriding the
org default — and you can have several webhooks for the same bot, each on
a different key (e.g. billing/quota separation per integration).

**Engine: already done.** `Resolve`'s `keyOverrides` (Pass 1) is the
mechanism. **Wiring: the piece to build** — the webhook `Config`
([pkg/webhooks/types.go:32](../pkg/webhooks/types.go#L32)) carries
`BotIDs`, `LaunchVars`, etc. but **no** key binding yet, and the
publisher passes `nil` overrides ([publisher.go:191](../pkg/server/cloudpublisher/publisher.go#L191)).

The build:
1. Add `KeyOverrides map[Provider]string` (provider → `key_id`) to
   `webhooks.Config` (+ bson/json, + CRUD validation that each `key_id`
   is same-tenant and matches the provider).
2. Thread it: webhook handler → `runview.LaunchSpec` (new field) →
   `RunMessage` → `resolveAndSealCredentials` → pass to `Resolve(...)`
   instead of `nil`.
3. Expose it on the webhook create/PATCH API + the studio webhook editor.

Validation must reject a `key_id` from another tenant or the wrong
provider (otherwise a webhook could pin a key it can't see).

## Multiple webhooks per bot — already supported

The webhook spine keys configs by `_id`, not by bot; nothing stops N
`webhook_configs` in one org all targeting the same `bot_ids`. Combined
with per-webhook overrides, that yields "same bot, different key per
webhook." No work needed beyond the override field above.

## Deployment guidance (cloud)

- **Proper model:** each org enters its keys via `POST /api/teams/{id}/api-keys`
  (sealed into Mongo, per-tenant). The publisher then resolves *that org's*
  keys per run.
- **Bootstrap shortcut (what ovh-dev used first):** the `iterion-llm`
  sealed Secret holds `ANTHROPIC_API_KEY` + `OPENAI_API_KEY` as pod env —
  a single instance-wide fallback. Fine to start, but it is **not**
  multi-tenant; once orgs bring their own keys it should shrink to (or be
  removed in favour of) the per-org store. Reseal/rotate playbook for the
  fallback lives in the k8s-deploy notes.

## Security invariants

1. A key's plaintext is sealed before it reaches Mongo and is only
   unsealed transiently to seal into a per-run bundle; it never lands in
   logs/events/artifacts/API responses (`json:"-"` on `sealed_secret`).
2. Resolution is tenant-scoped and fail-closed (`teamID` required;
   another user's personal key ranks 99 = never applies).
3. The agent never selects a key — selection is a server/publisher authz
   decision, mirroring the file-secret rule in [secrets.md](secrets.md).
4. A webhook override may only reference a key the webhook's tenant owns.

See also: [backends.md](backends.md) (backend/provider selection),
[secrets.md](secrets.md) (file/env/generic secrets), and the cloud
control-plane epic for the webhook spine.
