# mattermost-clarify

A Mattermost chat adapter for **@clarify-bot** — an ambient, read-only
discussion *facilitator* powered by iterion's
[`examples/clarify`](../../examples/clarify/main.bot) bot.

Mention `@clarify-bot` in a thread; from then on it watches that thread
and, when a message looks genuinely ambiguous, posts a concise
clarifying question or reformulation back into the thread. It only ever
sees messages from participants who have **explicitly consented**, and
everything it sends to the model is **anonymised**.

This service lives *outside* the iterion engine on purpose. iterion
ships a generic primitive — a **run-completion webhook** (see
[`docs/completion-webhooks.md`](../../docs/completion-webhooks.md)): you
launch a run with a `callback_url`, and iterion POSTs the final answer
there when the run finishes. This adapter is all the Mattermost-specific
glue around that primitive. A Slack adapter would implement the same
`ChannelDriver` interface and reuse every other file here.

## How it works

```
 Mattermost ──WS "posted"──▶ Coordinator ──┐
     ▲                          │           │ relevant + consented?
     │ consent buttons          │           ▼
     │ (REST)            ConsentStore   POST /api/runs  ──▶ iterion
     │                   Anonymizer     (callback_url, token)   │
     │                                                          │ run
     └──── reply (REST) ◀── HandleCompletion ◀── POST /callback ◀┘
                                              (run-completion webhook)
```

1. **Activation** — a post that `@`-mentions the bot marks *that thread*
   (its root post) active. Scope is the **thread**, never the channel.
2. **Consent** — the bot posts an interactive **Accept / Decline**
   prompt. A participant's messages are sent to the model only after
   they Accept. Non-consenting participants are **excluded entirely** —
   neither their content nor a placeholder reaches the model. New
   participants are prompted once, on their first post.
3. **Relevance pre-filter** — every new post in an active thread is run
   through a cheap [`RelevanceFilter`](filter.go). Only on a hit does the
   adapter launch a full clarify run. The default is a dependency-free
   heuristic (questions + confusion markers); see *Relevance filter*
   below for the recommended LLM upgrade.
4. **Run** — the adapter builds an anonymised, chronological transcript
   (consenting speakers only, stable `User A` / `User B` pseudonyms) and
   POSTs it to iterion with a `callback_url` back to itself and a
   `callback_token` encoding `{channel, root_id}`.
5. **Reply** — iterion fires the completion webhook; the adapter decodes
   the token and posts `final_answer` as a threaded reply. An empty
   answer (the facilitator choosing silence) posts nothing.

## Privacy model

- **Consent is mandatory and thread-scoped.** Default is exclusion:
  unknown and declined both mean "do not send". ([`consent.go`](consent.go))
- **Non-consenting messages never reach the model** — excluded at
  transcript-build time, with no placeholder. ([`anonymize.go`](anonymize.go))
- **Pseudonyms are stable per thread; the de-anonymisation table never
  leaves the process** — not in the transcript, not in the callback
  token, not sent to the model.
- **The callback token carries only routing ids** (channel, thread), no
  identity. ([`callbacktoken.go`](callbacktoken.go))

State is in-memory by design: a restart re-asks for consent, which is
the privacy-safe default.

## Configuration (env)

| Var | Meaning |
|-----|---------|
| `CLARIFY_LISTEN_ADDR` | adapter HTTP bind (default `:8090`) |
| `CLARIFY_CALLBACK_URL` | public URL iterion POSTs completion webhooks to, e.g. `https://adapter.example.com/callback` |
| `CLARIFY_ACTION_URL` | public URL Mattermost POSTs consent clicks to, e.g. `https://adapter.example.com/mm/actions` |
| `ITERION_BASE_URL` | iterion server base (default `http://localhost:8080`) |
| `ITERION_AUTH_TOKEN` | optional bearer for iterion's auth middleware |
| `CLARIFY_BOT_PATH` | path to the clarify bot (default `examples/clarify/main.bot`) |
| `CLARIFY_MODEL` | optional model override (`vars.model`) |
| `MM_HTTP_BASE` | Mattermost base, e.g. `https://mm.example.com` |
| `MM_WS_URL` | Mattermost WS, e.g. `wss://mm.example.com/api/v4/websocket` |
| `MM_BOT_TOKEN` | bot account access token |
| `MM_BOT_USER_ID` | bot's user id (used to detect mentions + ignore self) |
| `CLARIFY_WEBHOOK_SECRET` | shared HMAC secret authenticating the completion callback; **must equal `ITERION_COMPLETION_WEBHOOK_SECRET` on the iterion server**. Empty = callbacks unauthenticated (private-network-only) |
| `CLARIFY_MM_ACTION_TOKEN` | shared secret embedded in consent buttons and checked on `/mm/actions`; rejects forged consent clicks. Empty = unauthenticated |

## Endpoint authentication

The adapter exposes two inbound endpoints; both are authenticated when
their secret is configured (and log a startup warning when not):

- **`POST /callback`** (iterion → adapter). iterion signs each payload
  with `ITERION_COMPLETION_WEBHOOK_SECRET`; the adapter verifies the
  `X-Iterion-Signature` HMAC against `CLARIFY_WEBHOOK_SECRET` over the
  raw body and returns `401` on mismatch. Set the **same** value on both
  sides. See [docs/completion-webhooks.md](../../docs/completion-webhooks.md#authenticating-the-payload-hmac-signature).
- **`POST /mm/actions`** (Mattermost → adapter). The adapter embeds
  `CLARIFY_MM_ACTION_TOKEN` in each consent button's context; Mattermost
  echoes it back, and the adapter constant-time-compares it on receipt,
  returning `401` on mismatch. This stops a forged POST from flipping a
  user's consent.

Note the `callback_token` (which encodes `{channel, thread}`) is **not**
an auth mechanism — it is only for routing the reply to the right thread.
Authentication is the HMAC signature above.

Because the adapter and iterion typically run on the same private
network, set `ITERION_COMPLETION_WEBHOOK_ALLOW_PRIVATE=1` **on the
iterion server** so its SSRF guard permits the adapter's private
`callback_url`.

## Run

```bash
devbox run -- go build -o mattermost-clarify ./contrib/mattermost-clarify
CLARIFY_CALLBACK_URL=https://adapter.example.com/callback \
CLARIFY_ACTION_URL=https://adapter.example.com/mm/actions \
MM_HTTP_BASE=https://mm.example.com \
MM_WS_URL=wss://mm.example.com/api/v4/websocket \
MM_BOT_TOKEN=… MM_BOT_USER_ID=… \
./mattermost-clarify
```

In Mattermost: create a bot account, grant it the relevant channels,
and ensure interactive message buttons can reach `CLARIFY_ACTION_URL`.

## Relevance filter

The shipped default ([`heuristicFilter`](filter.go)) is intentionally
cheap and coarse — it never calls a model, so it works out of the box,
but it only catches questions and explicit confusion markers. The
recommended production setup replaces it with an **LLM-backed filter** (a
small/fast model) implementing the same one-method `RelevanceFilter`
interface, which judges genuine ambiguity far better. This is a
deliberate, documented limitation, not a silent cap.

## Status / testing

The privacy + orchestration cores are unit-tested
([`anonymize_test.go`](anonymize_test.go),
[`consent_test.go`](consent_test.go),
[`callbacktoken_test.go`](callbacktoken_test.go),
[`coordinator_test.go`](coordinator_test.go)) with a fake driver and
launcher.

The Mattermost I/O ([`mattermost.go`](mattermost.go)) follows
Mattermost's documented WebSocket envelope + REST shapes and compiles,
but **must be validated against a live Mattermost instance** — it is the
one part that cannot be unit-tested without a server.
