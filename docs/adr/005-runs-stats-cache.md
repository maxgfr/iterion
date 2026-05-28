# ADR-005: Runs-stats cache — lazy version-keyed memoization, not broker-push invalidation

- **Status**: Accepted
- **Date**: 2026-05-28
- **Authors**: devthejo
- **Code**: [pkg/server/runs_stats_cache.go](../../pkg/server/runs_stats_cache.go)
  (`runStatsCache`, `runVersion`),
  [pkg/server/runs_stats.go](../../pkg/server/runs_stats.go)
  (`cachedRunCostByDay`),
  [pkg/server/server.go](../../pkg/server/server.go) (`statsCache` field),
  [pkg/server/projects.go](../../pkg/server/projects.go) (clear on project switch)

## Context

The cross-run dashboard (`GET /api/v1/runs/stats`, studio `/insights`)
sums per-node `_cost_usd` by walking every recent run's `events.jsonl`
— one `store.ScanEvents` per run. The original handler did this on every
request and explicitly deferred caching in a code comment: *"if a store
grows past low thousands of recent runs we'll add a server-side
memoization keyed by (runID, updated_at)."* The studio multiplies the
cost: the window chips (7/14/30/90d) each trigger a fresh load over
largely-overlapping run sets.

The feature ask was: **"aggregation off the actor goroutine, cache +
invalidate on event-write."** Two facts in the existing code shaped the
design:

1. **`store.AppendEvent` does NOT bump `run.UpdatedAt`** (it only
   advances the per-run seq counter and writes the line). So a cache
   keyed purely on `UpdatedAt` would serve *stale cost for an in-flight
   run* that keeps appending `node_finished` events while `UpdatedAt`
   sits still between checkpoint saves.
2. **`runview.EventBroker` only supports per-`runID` subscriptions**
   (`Subscribe(runID)`), and exists to fan events out to WS clients. It
   has no "any event was written, anywhere" hook. A literal "invalidate
   on event-write" implementation would require widening that broker
   with a global observer and threading it into the cache.

Separately, the **"off the actor goroutine"** half was already satisfied:
`ScanEvents` opens the file and reads it holding **no** store lock, and
the handler runs on its own per-request goroutine — a dashboard scan
never contends with `AppendEvent`'s `s.mu`. No work was needed there; the
open question was purely *how to cache without serving stale numbers*.

## Decision

Use a **lazy, version-keyed, terminal-runs-only** memoization (pull), not
an active broker-push invalidation.

1. `runStatsCache` memoizes the single expensive, run-scoped output — the
   per-run **cost-by-day map**. Duration and status come from the
   in-memory `RunSummary` (already loaded by `ListCtx`) and are not
   cached.
2. **Only terminal runs are cached** (`RunStatus.IsTerminal()` —
   finished / failed / failed_resumable / cancelled). A terminal run
   appends no further events, so its cost-by-day map is immutable.
   Non-terminal runs (running / paused / queued) are **never** cached and
   are re-scanned on every request — they are few, and this keeps the
   live numbers honest.
3. The entry is keyed by `runID` with a **version** of
   `UpdatedAt.UnixNano() | Status`. The one path that mutates an
   already-terminal run — `iterion resume --force` re-finishing it —
   advances `UpdatedAt`, so it busts the cache automatically.
4. The expensive `ScanEvents` walk runs **outside** the cache mutex
   (`get` … scan … `put`); the lock guards only map access. Concurrent
   cold-cache loads at worst recompute the same value once.
5. The cache is **cleared on project switch** (under the same `stateMu`
   that swaps `s.runs`), since the run set changes wholesale.

The key is built from `RunSummary` fields, not a file stat, so the cache
is **store-agnostic** — it works unchanged for the Mongo/cloud store.

This satisfies "invalidate on event-write" *by construction*: a cached
entry corresponds, by the terminal-only rule, to a run that receives no
further writes. The implicit invariant replaces an explicit hook.

## Trade-offs

| Dimension | Lazy version-keyed, terminal-only (chosen) | Active broker-push invalidation (literal spec) |
|---|---|---|
| Correctness for in-flight runs | ✅ never cached → always fresh | ✅ (if hook fires reliably) |
| Broker surface | none — broker untouched | must add a global "all events" observer + lifecycle |
| Layering | cache is self-contained in `pkg/server` | couples WS fan-out infra to cache invalidation |
| Store-agnostic | ✅ uses `RunSummary` only | ✅, but hook lives in the local in-process broker only |
| Concurrency risk | mutex guards a map; scan is lock-free | publisher path now also touches the cache |
| Handles `resume --force` | ✅ via `UpdatedAt` version bust | needs the same version guard anyway |

The single concession: a cached terminal run whose `events.jsonl` is
mutated *without* an `UpdatedAt` change would serve a stale number. That
only happens if something rewrites a finished run's log out-of-band — not
a path the engine takes — and the version guard covers the one in-tree
case (`resume --force`).

## Alternatives considered

### 1. Active broker-push invalidation (the literal reading)
Subscribe to event writes and evict `cache[runID]` on each.
**Rejected**: `EventBroker` is per-`runID` and WS-oriented; a global hook
widens its API and lifecycle for **zero correctness gain** over
terminal-only caching (cached runs get no further events by definition).
It also bleeds cache concerns into the publisher hot path. Kept as a
named future seam, not a requirement.

### 2. Cache all runs keyed on `UpdatedAt`
**Rejected**: `AppendEvent` doesn't bump `UpdatedAt`, so an in-flight run
would serve stale cost between checkpoint saves — the exact failure the
cache must avoid.

### 3. Cache the whole `StatsResponse` keyed by `sinceDays`
**Rejected**: coarser granularity with no reuse across the 7/14/30/90d
window chips (each window is a separate key), and it still needs a global
"something changed" signal to invalidate. Per-run entries are
window-independent and shared across all four windows.

### 4. Key on the `events.jsonl` file stat (size/mtime)
**Rejected**: ties the cache to the filesystem store and breaks the
Mongo/cloud store, which has no local file to stat. `RunSummary` fields
keep it store-agnostic.

## Consequences

- **Terminal runs scan once per process.** The bulk of historical data
  (and the expensive scans) is read once and served from memory
  thereafter; all four window chips share the per-run entries.
- **Live runs stay correct.** Non-terminal runs are always re-scanned, so
  the dashboard reflects in-flight cost accurately.
- **`resume --force` is handled.** Re-finishing a run advances
  `UpdatedAt`, which busts the version key and forces a re-scan.
- **No broker change.** `EventBroker` stays a pure WS fan-out; cache
  invalidation is a property of the cache, not the event path.
- **"Off the actor goroutine" documented, not changed.** `ScanEvents`
  already holds no store lock and runs on the request goroutine; the
  cache's scan likewise runs outside its own mutex.
- **Memory.** One small `map[string]float64` per terminal run, cleared on
  project switch. Unbounded over a very long-lived single-project
  process; a soft size cap (or pruning entries not seen within the
  largest window) is a noted future safeguard, deliberately omitted for
  v1 because entries are tiny.
- **Future push invalidation is bounded and named.** If ever required, a
  global broker observer that drops `statsCache` entries is the single
  seam — but it is not needed for correctness.
