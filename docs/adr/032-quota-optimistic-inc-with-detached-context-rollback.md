# ADR-032: Quota uses optimistic increment with detached-context rollback

- **Status**: Accepted
- **Date**: 2026-06-22
- **Authors**: Adry
- **Code**: [pkg/orgusage/mongo.go](../../pkg/orgusage/mongo.go), [pkg/webhooks/mongo.go](../../pkg/webhooks/mongo.go)

## Context

Per-org monthly run and webhook caps must be enforced under concurrency. A denied request must not consume quota, and a cancelled request must not leave an optimistic quota increment behind.

The counter is Mongo-backed, so the implementation needs to use document-level atomic operations rather than an in-process lock that would not cover multiple server instances.

## Decision

The org-usage counter in [`pkg/orgusage/mongo.go`](../../pkg/orgusage/mongo.go) uses `FindOneAndUpdate` with `$inc`, `upsert`, and `ReturnDocument(After)`. It inspects the post-increment document to decide whether run or cost caps are exceeded. If the cap is breached, it rolls back the increment with an `UpdateOne` executed under `context.WithoutCancel(ctx)`.

The webhook counter in [`pkg/webhooks/mongo.go`](../../pkg/webhooks/mongo.go) follows the same pattern for org and per-webhook monthly counters: bump first, inspect the post-bump count, and compensate with a detached rollback when the cap is exceeded or the second-stage per-webhook bump fails.

The allow/deny decision is atomic at the counter-document update boundary. Rollback is compensation for the denied unit, not a separate pre-check protocol.

## Trade-offs

| Dimension | Optimistic `$inc` then rollback | Pessimistic find-check-increment |
|---|---|---|
| Concurrency | Atomic increment establishes a single post-update count. | Separate read and write races unless wrapped in heavier locking/transactions. |
| Contention | One hot atomic update plus rare compensation. | More coordination around every allowed request. |
| Denied requests | Need rollback compensation. | No rollback if the pre-check is correct. |
| Cancellation safety | Uses `context.WithoutCancel` for rollback. | Fewer compensation calls, but still race-prone. |

The honest concession is that denied over-cap requests briefly increment the counter and rely on best-effort compensation.

## Alternatives considered

### 1. Find, check, then increment

The counter could have read the current monthly value, compared it with limits, and incremented only when under cap.

**Rejected because**: concurrent callers can all observe the same under-cap value and then over-admit unless the code adds heavier locking or transactions.

### 2. Use request context for rollback

Rollback could have reused the original request context for cancellation propagation.

**Rejected because**: a cancelled or timed-out request is exactly when leaked quota is most likely; compensation must outlive request cancellation.

## Consequences

- **Concurrent admissions use Mongo atomicity.** The post-increment document is the source of truth for cap decisions.
- **Denied calls do not intentionally consume quota.** Breached caps trigger a compensating decrement.
- **Rollback survives client cancellation.** `context.WithoutCancel` prevents cancelled requests from permanently inflating counters.
- **Counters may be briefly over the cap.** During the window before compensation lands, reads can observe an optimistic increment.
- **Rechallenge for counter-service migration.** If quotas move to a dedicated counter service with compensation transactions, this Mongo-specific pattern should be reconsidered.
