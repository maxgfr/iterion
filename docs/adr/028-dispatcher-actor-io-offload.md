# ADR-028 — Dispatcher actor: offload blocking tracker I/O

Status: Accepted (roadmap). This ticket implements **Step 1 only**.

## Context
The dispatcher is an actor: a single goroutine (`actorLoop`, `pkg/dispatcher/dispatcher.go`) owns all mutable state (`c.state`); other goroutines send typed commands on `c.cmds`. No locks, deterministic ordering — deliberately simple.

The cost: everything the actor does is serialized, so any blocking call it makes blocks the whole dispatcher. The actor makes synchronous tracker HTTP calls in three places, all on the actor goroutine:
- `tick` → pollOnce: `tracker.ListCandidates` / `RefreshStates` (`pkg/dispatcher/loop.go`)
- `dispatch` (per eligible issue, serially): `tracker.Claim`, `UpdateState`, and on error `revertTransition` / `Release`
- `finishRun` (cmdRunFinished): `Release`, `revertTransition` (`pkg/dispatcher/commands.go`)

While the actor is blocked on any of these, it processes no other command — snapshot requests (the studio dashboard), cmdEvent, and cmdRunFinished all queue behind it.

This is currently **not** a deadlock: the `cmds` buffer is sized to MaxConcurrent (worker cmdRunFinished bursts never fill it) and the tracker HTTP client has timeouts (a hung call fails, it doesn't hang forever). The residual symptom is **latency / unresponsiveness** during slow tracker I/O — most visibly, the studio dashboard snapshot lagging during a poll.

## Decision
Incrementally offload tracker I/O off the actor so the actor only ever does fast in-memory state mutations. Enabling insight: **the tracker is already the claim authority** — `tracker.Claim` is an atomic CAS that returns `ErrClaimConflict`. So the actor does not need to serialize claims for correctness; it only needs slot-accounting (concurrency caps), which is in-memory and fast. That makes the offload natural rather than bolted-on, and the conflict path already exists today.

Model the per-issue lifecycle as an explicit state machine — candidate → claiming → claimed → running → finishing → done — so the async intermediate states are first-class, enumerable and testable rather than implicit races.

Incremental sequence, safest first (each step independently mergeable + tested):
1. **Read-path decouple** — publish an immutable snapshot atomically; snapshot reads become lock-free and never wait on the actor's I/O. **(THIS TICKET)**
2. Discovery — push over poll where the mechanism exists (native fsnotify watcher; the inbound-webhook spine); where polling remains, run it in a side goroutine that posts a candidates command (read-only, lowest-risk; stale candidates are already tolerated).
3. finishRun Release/revert — already on a decoupled context; move just the HTTP to a tracked worker after the in-memory slot-free.
4. Claim/dispatch — optimistic claim: the actor reserves a slot, a worker does `Claim` (HTTP), `ErrClaimConflict` releases the slot. Hardest; do last, most tests.

### Alternatives rejected
- **Full big-bang offload now**: speculative for a per-host poller that is not the scale bottleneck (cloud scale rides the NATS queue + runner pods, not this actor); high blast radius on the daemon that drives every run.
- **Per-call timeout only**: bounds the violation but doesn't restore the "actor never blocks" invariant; acceptable as a stopgap, not the durable answer.
- **Do nothing**: latency persists and the invariant stays silently violated.

### Trigger to advance past Step 1
An observed symptom: dashboard snapshot lag during polls, per-state concurrency starved by serial claims, or a flaky tracker freezing the actor. Until then, Steps 2–4 stay documented future work.

## Consequences
- Step 1 removes the most visible symptom (dashboard lag) at low risk and lock-free.
- The durable direction is recorded; the heavy steps are deferred behind an explicit trigger.
- Each future step composes along existing invariants (tracker = claim authority; explicit state machine).
