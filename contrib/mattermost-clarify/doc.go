// Command mattermost-clarify is a chat channel adapter that wires a
// Mattermost bot ("@clarify-bot") to iterion's run-completion webhook
// (pkg/notify), letting iterion facilitate a thread without iterion
// itself knowing anything about Mattermost.
//
// It is deliberately OUT of the engine: iterion ships the generic
// primitive (POST /api/runs with a callback_url; a completion webhook
// fired at terminal state), and this adapter holds every
// platform-specific concern — Mattermost's WebSocket, interactive
// consent buttons, thread routing, anonymisation, and the relevance
// pre-filter. The engine stays generic (see CLAUDE.md "the engine
// stays generic"); a Slack adapter would implement the same
// ChannelDriver interface against this same package's cores.
//
// # Behaviour
//
// @clarify-bot is a THREAD-scoped ambient facilitator. Once mentioned
// in a thread it becomes active for that thread (root post id) only —
// never the whole channel. From then on it observes every new post in
// the thread and decides, per message, whether to respond:
//
//  1. Consent. On activation the bot posts an interactive consent
//     prompt (Accept / Decline buttons) into the thread. A participant's
//     messages are sent to the LLM ONLY after they have accepted.
//     Non-consenting participants' messages are excluded entirely from
//     the transcript (see Anonymizer + ConsentStore). New participants
//     are re-prompted on their first post.
//
//  2. Relevance pre-filter. Each new post from the active thread is run
//     through a cheap RelevanceFilter (default: a dependency-free
//     heuristic; production: plug an LLM-backed filter implementing the
//     same interface). Only if it judges the message worth a response
//     does the adapter launch the full `clarify` facilitator run.
//
//  3. Run. The adapter builds an anonymised transcript (consenting
//     speakers only, stable "User A/B" pseudonyms, chronological) and
//     POSTs it to iterion's launch API with a callback_url back to this
//     adapter and a callback_token encoding {channel, root_id}.
//
//  4. Reply. iterion fires the completion webhook at terminal state.
//     If final_answer is non-empty the adapter posts it as a reply in
//     the originating thread (root_id decoded from the token). An empty
//     answer — the facilitator choosing silence — posts nothing.
//
// # Privacy
//
// The de-anonymisation table (pseudonym → real user id) NEVER leaves
// this process: it is not part of the transcript, not sent to the LLM,
// and not echoed in the callback token. The token carries only routing
// ids (channel, thread), not identities.
package main
