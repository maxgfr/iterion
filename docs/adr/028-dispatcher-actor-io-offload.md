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

## 2026-06-17 — Step 2 implemented (candidate discovery offloaded)

`tracker.ListCandidates` — "the slowest synchronous step inside the actor
goroutine" — now runs on a short-lived goroutine (`launchDiscovery`,
`pkg/dispatcher/loop.go`) instead of inline in `tick()`. The goroutine does
only the HTTP/I/O and posts the result back as `cmdCandidates`
(`pkg/dispatcher/commands.go`); the actor runs the unchanged in-memory
sort / dispatch-skip prune / per-issue `dispatch` logic in
`cmdCandidates.apply`. The actor keeps draining `cmdRunFinished` /
`cmdEvent` / `cmdCancel` while a poll's discovery is in flight. Steps 3
(`finishRun` Release/revert) and 4 (claim/dispatch offload) remain future
work; `RefreshStates` is still synchronous on the actor (a later
sub-step of discovery, deliberately not touched here).

Implementation choices and their trade-offs:

- **Single-flight via a plain `bool`, skip not queue.** The discovery
  goroutine MUST NOT touch `c.state` (actor-only); the only shared
  bookkeeping is `state.discoveryInFlight`, set in `tick()` and cleared in
  `cmdCandidates.apply` — both on the actor goroutine, so a plain bool is
  race-free (no atomic). When a tick fires while a discovery is still in
  flight it *skips* launching a second one rather than queueing it. The
  rejected alternative — let ticks stack discoveries — risks unbounded
  goroutines + redundant tracker HTTP against a flaky/slow tracker, for no
  gain: a poller only needs the *latest* candidate set, and the next tick
  re-evaluates after the in-flight one posts.

- **Re-check pause + cost-cap gates in `cmdCandidates.apply`.** Discovery
  is now asynchronous, so the pause/cost-cap state the actor checked before
  launching it can flip during the I/O window — the old fully-synchronous
  `tick()` held them constant across the whole poll. We re-read the two
  cheap gates before dispatching so we never dispatch into a state the
  operator just paused/capped. Concurrency is *not* re-gated separately —
  the dispatch loop's own `MaxConcurrent` / `hasSlot` / `isClaimed` checks
  already re-validate it per issue. Accepted trade-off: candidates are a
  beat stale by the time they're dispatched, which ADR §Step 2 already
  declared tolerable (the tracker `Claim` CAS remains the conflict
  authority).

- **Discovery goroutines tracked on `workersWG`.** Reusing the existing
  worker WaitGroup (rather than a new one) means `Stop()` already drains
  them; the `cmds`-send is guarded by `c.stop` so a discovery that finishes
  after the actor exits never leaks on a blocked send.

Anti-façade test: `TestActorResponsiveWhileDiscoveryInFlight`
(`pkg/dispatcher/dispatcher_test.go`) gates the fake tracker's
`ListCandidates` on a channel, lets the kick-off tick hand discovery to the
side goroutine, and asserts the actor applies a `cmdReload` posted on
`c.cmds` (republishing the snapshot) within a tight deadline *while*
`ListCandidates` is still blocked — which fails before this change because
the actor was parked inside the synchronous call.

## 2026-06-17 — Step 3 implemented (finishRun tracker HTTP offloaded)

`finishRun` (`pkg/dispatcher/commands.go`) used to interleave fast in-memory
`c.state` mutations with the run's blocking tracker HTTP — `tracker.Release`
plus exactly one of the clean-finish transition (`maybeTransitionToCompleted`),
the cancel/retry revert (`revertTransition`), or the exhausted-failure give-up
move — all on the actor goroutine. While the actor was parked in that HTTP it
processed no other command.

Now the actor does ALL the state work synchronously (free the slot, clear or
schedule retries, decide the give-up-vs-retry outcome) and computes a
value-copy **`finishPlan`** capturing the immutable inputs the HTTP needs
(issueID, identifier, running-state target, completed-state target, source
state, failed state, attempt/run for logging). A tracked worker
(`launchFinish` → `runFinishWorker`) then executes the plan's tracker calls off
the actor, using the same background-derived 5s context so Release/transition
survive run-context / shutdown cancellation. The worker reads ONLY the plan plus
dispatcher-immutables (`c.tracker`, `c.logger`, `c.hostMarker`) — it never
touches `c.state`, and it never re-reads `c.cfg` for finishRun transition
targets. The actor remains the sole writer of `c.state`, and its finish-time cfg
snapshot remains authoritative even if a Reload is processed while the finish
worker's HTTP is in flight. Steps 4 (claim/dispatch offload) and
`RefreshStates`/`refreshRunningStates` remain future work, deliberately
untouched here.

Implementation choices and their trade-offs:

- **Transition first, `Release` last (reordered).** finishRun previously ran
  `Release` *before* the transition, but synchronously — no tick could
  interleave. Off the actor, releasing first would briefly leave a
  cleanly-finished issue *released + still in RunningState* = an eligible,
  unclaimed candidate, opening a spurious re-dispatch window (a tick's
  discovery could see it and `Claim` it for a duplicate run). Doing the
  transition first keeps the tracker claim held — so `ListCandidates` filters
  the issue — until it has been moved to its final, mostly-non-eligible state;
  `Release` runs last. This is invisible to existing assertions, which key on
  `UpdateState` call order/counts (`Release` is not an `UpdateState`).

- **Give-up uses an optimistic retry as the in-memory guard.** The give-up
  decision's fallback — "if the board can't represent FailedState, keep
  retrying rather than freeze the ticket" — is HTTP-result-dependent and must
  be preserved. The former `giveUpIfExhausted` is split into a pure in-memory
  predicate `exhausted(r)` (max-attempts + FailedState-set gate, decided on the
  actor) and the off-actor move. When exhausted, the actor **schedules the
  retry synchronously**; that retry entry is the re-dispatch guard
  (`isClaimed`) that blocks a tick from re-picking the issue for the whole
  worker HTTP window. The worker attempts the FailedState move: on success it
  posts `cmdDropRetry` so the actor drops the guard (give-up is final); on
  failure it reverts and leaves the retry in place — reproducing the legacy
  fallback exactly (same attempt count + backoff). A tombstone can't serve as
  the guard because `cmdRunFinished.apply` deletes tombstones immediately after
  `finishRun` returns. The only artifact is a transient "retry queued" log
  before "gave up" in the give-up-success case — informative, not wrong.

- **Finish workers tracked on `workersWG`.** Reusing the existing worker
  WaitGroup means `Stop()` already drains them; the `cmdDropRetry` send is
  guarded by `c.stop` (via `postCmd`) so a worker finishing after the actor
  exits never leaks on a blocked send.

Anti-façade tests (`pkg/dispatcher/dispatcher_test.go`):
`TestActorResponsiveWhileFinishHTTPInFlight` gates the fake tracker's
`Release` on a channel, drives a run to a clean finish so the finish worker
parks in `Release`, asserts the slot is already freed, and proves the actor
applies a `cmdReload` posted on `c.cmds` *while* `Release` is still blocked.
`TestSlotFreedBeforeReleaseHTTP` asserts `Snapshot().Slots.GlobalUsed` drops to
0 while `Release` is still in flight — slot accounting decoupled from the
release HTTP. `TestFinishRun_GiveUpMovesToFailedState` and
`TestFinishRun_GiveUpFallsBackToRetryWhenMoveRejected`
(`pkg/dispatcher/commands_test.go`) cover both give-up worker outcomes.

## 2026-06-17 — Step 4 implemented (reduced/safe variant: post-claim dispatch I/O offloaded, claim stays atomic)

`dispatch` (`pkg/dispatcher/loop.go`) used to run the post-claim setup I/O —
the in-progress `tracker.UpdateState` transition and `workspaces.Create` — inline
on the actor between `tracker.Claim` and the run-worker spawn. The `UpdateState`
HTTP can be slow (and a flaky tracker freezes it), parking the actor.

Now the actor, immediately after a confirmed `Claim`, allocates the running
entry + concurrency slot synchronously (`setupPending=true`,
`TransitionedFromState` not-yet-known) and hands the `UpdateState` + `Create` to a
tracked worker (`launchDispatchSetup` → `runDispatchSetup`). On success the
worker posts `cmdDispatchSetupDone{transitionedFrom}` (the actor records the
transition source + clears `setupPending`) and continues into `runWorker` on the
same goroutine; on `Create` failure it posts `cmdDispatchSetupDone{err}`, whose
apply reuses the Step-3 `finishRun` teardown. The actor's reapers
(`refreshRunningStates`, `reconcileStalled`) skip `setupPending` entries so the
in-flight transition isn't misread as an external move and a not-yet-started run
isn't stall-reaped on its claim-time watermark.

### Decision: keep `Claim` atomic on the actor — NOT the ADR's original "optimistic claim"

The original Step 4 above proposed **optimistic claim**: the actor reserves a
slot, a worker does `Claim`, and `ErrClaimConflict` releases the slot. We
deliberately did **not** do that. The trade-off:

- **Optimistic claim (rejected).** Moves the `Claim` HTTP off the actor too, but
  introduces a *reserved-but-not-yet-claimed* slot on the dispatch entry point —
  the highest-blast-radius path, the one that gates every run. That speculative
  reservation can leak (worker dies between reserve and conflict-release) and
  forces the slot/cap accounting to reason about a third, transient state.
- **Reduced variant (chosen).** Keep `Claim` atomic on the actor — it is already
  the conflict authority, and `ErrClaimConflict` simply skips the issue with **no
  slot allocated** (cheap, in-memory, no HTTP-shaped blocking on the conflict
  path). Offload only the work that happens *after* a confirmed claim. The slot
  is allocated post-claim, so concurrency stays enforced from claim time and
  there is no speculative reservation to leak. We give up moving the `Claim` HTTP
  itself off the actor — an acceptable residual, since `Claim` is a single fast
  CAS, not the slow `UpdateState`/`Create` pair this step targets.

### Implementation choices and their trade-offs

- **Setup failure folds into `cmdDispatchSetupDone{err}`, not a bare
  `cmdRunFinished`.** `finishRun`'s revert reads `r.TransitionedFromState` off the
  entry, so the transition the worker performed off-actor MUST be recorded on the
  entry *before* the teardown runs. Routing the failure through the same command
  (which sets `TransitionedFromState` then calls `finishRun`) keeps "record then
  revert" atomic in one actor-side apply, avoiding a two-command ordering
  dependency. It still **reuses** the Step-3 teardown — it does not duplicate
  slot-free / revert / release / retry.

- **Slot allocated at claim time, before the off-actor setup.** The running entry
  + `slotsByState++` land on the actor the instant `Claim` succeeds, so
  `MaxConcurrent` / per-state caps hold from claim time and `isClaimed` / the
  dispatch loop won't re-pick the issue while setup is in flight — no
  over-dispatch window.

- **Workspace path computed on the actor.** `Workspaces.Path(issueID)` is
  deterministic (it's the directory `Create` materialises), so the entry/spec
  carry the path without waiting on the off-actor `Create`; the worker's `Create`
  only does the mkdir + reports `created` (gating the after_create hook) and
  surfaces disk errors. The setup worker never touches `c.state` or `c.cfg` —
  cfg-derived inputs (`runningTarget`) are captured into the value-copy
  `dispatchSetupPlan` on the actor.

- **One `workersWG` slot for setup+run.** `launchDispatchSetup` adds a single
  worker-group entry that covers both the setup I/O and the folded run, so
  `Stop()` drains it; its panic-recovery guards ONLY the setup I/O (`runWorker` is
  invoked outside it, preserving its existing panic semantics).

Anti-façade tests: `TestActorResponsiveWhileDispatchSetupInFlight`
(`pkg/dispatcher/loop_setup_offload_test.go`) gates the fake tracker's
`UpdateState` and proves the actor applies a `cmdReload` while the transition is
blocked. `TestDispatch_ClaimConflictAllocatesNothing` proves an
`ErrClaimConflict` records no entry, consumes no slot, performs no transition,
and launches no worker. `TestDispatch_SlotCountedFromClaimTime` proves the cap
holds from claim time (one slot used, one running entry, under
`MaxConcurrent=1` with two ready issues while setup is gated).
`TestDispatch_RevertsOnWorkspaceCreateFailure` (`pkg/dispatcher/loop_state_test.go`)
proves a `Create` failure reuses the `finishRun` teardown (slot freed,
transition reverted, claim released, retry scheduled). The optimistic-claim
offload and `RefreshStates`/`refreshRunningStates` offload remain future work.
