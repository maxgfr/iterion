import { useCallback, useEffect, useRef, useState } from "react";

import { getRun } from "@/api/runs";
import { errorMessage } from "@/lib/errorHints";
import { useRunStore } from "@/store/run";
import { useUIStore } from "@/store/ui";

// useRunSnapshot owns the run-console's REST snapshot fetch + event-
// history hydration on run open. Both have non-trivial retry/abort
// semantics that the host file should not have to carry inline.
//
// Snapshot fetch (with retry on 404):
//   The launch API returns the run_id as soon as the engine goroutine
//   is scheduled, but the goroutine still needs a beat to call
//   store.CreateRun before run.json exists on disk. Fetching too early
//   therefore 404s, and without a retry the page gets stuck in the
//   skeleton until the user reloads — the WS path was supposed to fill
//   the gap but doesn't always push the initial snapshot eagerly. A
//   short backoff loop closes the race for the common case (run.json
//   typically lands within ~50–200ms) without papering over a genuinely
//   missing run.
//
//   The fetch loop is exposed via a callback so both the initial mount
//   effect AND the user-facing Retry button on RunViewLoadError can
//   re-trigger it in place — no window.location.reload() (which would
//   destroy tabs, scroll position, and chat dock state). The
//   loadAbortRef holds the cancel handle of the in-flight attempt so a
//   new fetch (or unmount) bails the previous loop before starting
//   fresh, preventing two retry budgets from racing.
//
// Event-history hydration:
//   RunMetrics (always-visible header strip) folds cost + llm_step
//   counts from the events array, and ReportTab does the same for the
//   cost breakdowns — both render the empty state when no events are
//   loaded. The action dedupes per run via historyFetchedForRun, so
//   this stays cheap on re-renders and tab toggles. On failure, we
//   surface a *persistent* toast with a Retry action so the operator
//   can re-attempt in place instead of having to close and re-open the
//   run. loadEventHistoryIfMissing rolls back its historyFetchedForRun
//   marker on failure, so re-invoking it genuinely retries the fetch.
//
// refreshSnapshot — used by post-merge UI to refetch run.json so
// RunHeader and the merge-state-driven UI catch up after a Commits-tab
// merge action lands. The WS pushes events but not run-meta updates,
// so a manual REST fetch is the simplest path.
export interface RunSnapshotHandle {
  loadFailed: { status: number; message: string } | null;
  handleRetryLoad: () => void;
  refreshSnapshot: () => void;
}

export function useRunSnapshot(runId: string | null): RunSnapshotHandle {
  const applySnapshot = useRunStore((s) => s.applySnapshot);
  const loadEventHistoryIfMissing = useRunStore(
    (s) => s.loadEventHistoryIfMissing,
  );
  // Tracks whether the initial snapshot fetch has exhausted its retries
  // without success. Flipped true so the skeleton swaps for a clear
  // "Run not found" message instead of pulsing forever. Distinguishes
  // "loading" (snapshot null + !loadFailed) from "no such run on this
  // daemon" (snapshot null + loadFailed). Reset on runId change.
  const [loadFailed, setLoadFailed] = useState<
    { status: number; message: string } | null
  >(null);

  const loadHistory = useCallback(() => {
    if (!runId) return;
    loadEventHistoryIfMissing(runId).catch((err) => {
      console.warn("[run] event history hydration failed:", err);
      const msg = errorMessage(err);
      useUIStore.getState().addToast(
        `Couldn't load event history: ${msg}`,
        "error",
        { persistent: true, action: { label: "Retry", onClick: () => loadHistory() } },
      );
    });
  }, [runId, loadEventHistoryIfMissing]);
  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  const loadAbortRef = useRef<(() => void) | null>(null);
  const fetchSnapshot = useCallback(() => {
    if (!runId) return;
    // Cancel any in-flight retry loop before kicking off a new one so
    // a Retry click (or a runId change) can't leave the previous loop
    // ticking against the network in the background.
    loadAbortRef.current?.();
    let cancelled = false;
    let attempt = 0;
    let timerId: ReturnType<typeof setTimeout> | null = null;
    setLoadFailed(null);
    const fetchWithRetry = () => {
      getRun(runId)
        .then((snap) => {
          if (!cancelled) applySnapshot(snap);
        })
        .catch((err: Error) => {
          if (cancelled) return;
          attempt += 1;
          const msg = err?.message ?? "";
          const is404 = msg.includes("API error 404");
          const cap = is404 ? 3 : 20;
          if (attempt < cap) {
            // Track the timer so the cleanup can cancel it. The
            // prior implementation only flipped `cancelled` for the
            // setState path; the timer kept firing for the full
            // retry budget after navigation, hammering the network.
            timerId = setTimeout(() => {
              timerId = null;
              if (!cancelled) fetchWithRetry();
            }, 250);
          } else if (!cancelled) {
            setLoadFailed({ status: is404 ? 404 : 0, message: msg });
          }
        });
    };
    loadAbortRef.current = () => {
      cancelled = true;
      if (timerId != null) {
        clearTimeout(timerId);
        timerId = null;
      }
    };
    fetchWithRetry();
  }, [runId, applySnapshot]);
  useEffect(() => {
    fetchSnapshot();
    return () => {
      loadAbortRef.current?.();
      loadAbortRef.current = null;
    };
  }, [fetchSnapshot]);
  const handleRetryLoad = useCallback(() => {
    fetchSnapshot();
  }, [fetchSnapshot]);

  const refreshSnapshot = useCallback(() => {
    if (!runId) return;
    getRun(runId)
      .then(applySnapshot)
      .catch(() => undefined);
  }, [runId, applySnapshot]);

  return { loadFailed, handleRetryLoad, refreshSnapshot };
}
