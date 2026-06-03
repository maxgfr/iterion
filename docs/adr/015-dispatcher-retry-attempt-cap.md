# ADR-015: Dispatcher retry attempt cap — bounded retries that give up into a terminal board state

- **Status**: Accepted
- **Date**: 2026-06-02
- **Authors**: devthejo
- **Code context**: [`pkg/dispatcher/commands.go`](../../pkg/dispatcher/commands.go)
  (`finishRun` default branch, `giveUpIfExhausted`),
  [`pkg/dispatcher/retry.go`](../../pkg/dispatcher/retry.go) (`scheduleRetry`
  attempt accumulation), [`pkg/dispatcher/config.go`](../../pkg/dispatcher/config.go)
  (`AgentConfig.MaxAttempts`, `AgentConfig.FailedState`, defaults),
  [`pkg/dispatcher/native/board.go`](../../pkg/dispatcher/native/board.go)
  (`StateBlocked`, the default terminal column the give-up targets).
  Related: [ADR-009 daily spend cap](009-daily-spend-cap.md) (the only prior
  backstop against runaway dispatcher spend).

## Context

The dispatcher retried a failed run forever. `finishRun`'s non-cancellation
branch always called `scheduleRetry`, and `scheduleRetry` only capped the
backoff *duration* (`MaxRetryBackoffMS`, default 5 min); it had no ceiling on
the *number* of attempts. `EngineRunner.Dispatch` returns the engine error
verbatim, so any deterministically-failing run — a missing/typo'd bot, a
schema-validation error, a persistent provider error after the recovery
dispatch exhausts, an intentional `FailNode` — re-dispatched every ≤5 minutes
indefinitely.

The operator-facing damage:

- **Silent on the board.** A failed dispatch reverts the issue to its source
  state, so the card just bounces `ready → in_progress → ready`. The failure
  was visible only in the dispatcher dashboard's retries table — never on the
  board the operator actually watches.
- **Unbounded spend.** The only backstop was `limits.max_cost_per_day_usd`
  (ADR-009), which defaults to **0 = disabled**. So the out-of-the-box config
  burned model spend forever on a ticket that could never succeed.

While fixing this we found a **latent bug** that the cap depends on:
`scheduleRetry` derived `prevAttempt` from `c.state.retries[issueID]`, but
`dispatch()` deletes that entry when it picks the retry up (carrying the count
onto the `runningEntry`). So the lookup always missed and the attempt counter
reset to 1 every cycle — the dashboard never advanced past "attempt 1", and a
naive cap keyed on it would never fire.

## Decision

Add `agent.max_attempts` (default **10**) capping the *total* dispatch
attempts (initial run + retries). When a run fails and the attempts made
reach the cap, the dispatcher **gives up** instead of rescheduling: it moves
the issue to a terminal `agent.failed_state` (default **`blocked`**, a
terminal column on the default board) and drops the retry bookkeeping. The
issue thereby (a) becomes visible on the board in the Blocked column and
(b) leaves the eligible set so it stops being re-dispatched.

Supporting changes:

- `scheduleRetry` now reads `prevAttempt` from the `runningEntry`
  (`prev.Attempt`), which accumulates correctly, so both the cap and the
  dashboard attempt counter work.
- A negative `max_attempts` is the explicit "retry forever" escape hatch
  (mapped to 0 = no cap).
- `failed_state: none` opts out of the terminal move.

### Alternatives rejected

1. **Keep unbounded retries, only surface the loop in the dashboard.**
   Rejected: it doesn't stop the spend bleed, and the board — the operator's
   primary surface — still shows nothing. The reported blocker is precisely
   that the *default* config fails silently and expensively.
2. **Default `max_attempts` to 0/unbounded (pure opt-in).** Rejected: that
   leaves the default config exactly as broken; the whole point is to make the
   safe behaviour the default. Operators who genuinely want infinite retries
   (e.g. "retry until I fix the host") opt in with a negative value.
3. **Track "given up" issues in a dispatcher-internal set instead of a board
   state.** Rejected: it's invisible to the operator and not durable across a
   daemon restart — the issue would silently vanish from dispatch with no
   on-board signal, trading one silent failure for another.
4. **Stop retrying but leave the issue in its (eligible) source state.**
   Rejected: an eligible issue is re-listed by `ListCandidates` on the next
   tick, so the loop simply resumes. Giving up *requires* moving to a
   non-eligible state.
5. **Special-case `FailNode` (never retry a deliberate failure).** Deferred,
   not rejected: it needs typed runtime errors to distinguish an intentional
   terminal failure from a transient one. The attempt cap already bounds the
   `FailNode`-retry symptom safely; the precise classification is left as a
   follow-up.

The non-obvious trade-off is the **graceful fallback**: if the `failed_state`
move is unavailable (the board doesn't define the state, or the tracker
rejects / doesn't support the transition), `giveUpIfExhausted` returns `false`
and the dispatcher keeps the legacy retry behaviour rather than freezing the
ticket in a non-terminal state it can't escape. We chose "never strand an
issue" over "always enforce the cap": on a misconfigured board the cap simply
doesn't engage (logged loudly), which is no worse than today, whereas a hard
stop without a terminal target would lose the work.

## Consequences

- The default config now bounds a doomed ticket to ~10 attempts (tens of
  minutes at the capped backoff) and surfaces it in the **Blocked** column,
  instead of retrying forever invisibly.
- **Behaviour change:** a sustained provider outage can now push in-flight
  issues to `blocked` after the cap; the operator re-opens them once the host
  recovers. This is the deliberate trade for bounded spend; raise
  `max_attempts` or set it negative to restore unbounded retries.
- The dashboard attempt counter and `RetryView.Attempt` now reflect the real
  attempt number (previously frozen at 1).
- Boards without a `blocked` (or configured `failed_state`) column keep the
  prior unbounded-retry behaviour, with a warning logged on each exhaustion.
- Operator visibility improves without a frontend change: failures land in an
  existing board column. A success/failure badge on the per-issue "last run"
  link (the remaining presentational half) is left as a follow-up finding.
