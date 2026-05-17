import { useEffect, useMemo, useRef, useState } from "react";

import { listRuns, type RunStatus, type RunSummary } from "@/api/runs";

// Cheap fingerprint of a run list: each run reduces to its
// status-affecting fields. If two consecutive polls produce the same
// fingerprint, we skip the setState to avoid re-rendering every
// consumer (HomeView's banner + recent panel re-filter on every poll
// otherwise, even when nothing changed).
function fingerprint(runs: RunSummary[]): string {
  const parts: string[] = [];
  for (const r of runs) {
    parts.push(
      `${r.id}:${r.status}:${r.updated_at}:${r.finished_at ?? ""}:${r.queue_position ?? ""}`,
    );
  }
  return parts.join("|");
}

const POLL_INTERVAL_FAST_MS = 3000;
const POLL_INTERVAL_SLOW_MS = 8000;
// Above this many queued runs we slow polling to relieve the cloud
// server. Mirrors RunListView's contract — see cloud-ready plan §F.
const QUEUED_BACKOFF_THRESHOLD = 10;

export function computePollingInterval(
  counts: Partial<Record<RunStatus, number>>,
): number {
  const queued = counts.queued ?? 0;
  return queued >= QUEUED_BACKOFF_THRESHOLD
    ? POLL_INTERVAL_SLOW_MS
    : POLL_INTERVAL_FAST_MS;
}

export interface UseRunsOptions {
  status?: RunStatus | "";
  limit?: number;
}

export interface UseRunsResult {
  runs: RunSummary[];
  counts: Partial<Record<RunStatus, number>>;
  loading: boolean;
  error: string | null;
}

// Polls the runs list at an adaptive interval (3s normally, 8s when the
// queue is deep). One hook instance per consumer; siblings on the same
// view should be fed from a shared parent invocation so we don't fan
// out duplicate requests.
export function useRuns(opts: UseRunsOptions = {}): UseRunsResult {
  const { status = "", limit } = opts;
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const lastFingerprint = useRef<string>("");

  const counts = useMemo(() => {
    const m: Partial<Record<RunStatus, number>> = {};
    for (const r of runs) m[r.status] = (m[r.status] ?? 0) + 1;
    return m;
  }, [runs]);

  const pollMs = computePollingInterval(counts);
  // Hold the latest pollMs in a ref so we can vary the next-tick
  // delay without ripping down the polling effect every time the
  // queue depth crosses the threshold. The previous version had
  // pollMs in the effect deps, which on a fast→slow transition
  // would discard a freshly-scheduled tick and wait the full slow
  // interval before the next fetch.
  const pollMsRef = useRef(pollMs);
  useEffect(() => {
    pollMsRef.current = pollMs;
  }, [pollMs]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;
    const fetchRuns = async () => {
      try {
        const out = await listRuns({
          status: status || undefined,
          limit,
        });
        if (cancelled) return;
        const fp = fingerprint(out);
        if (fp !== lastFingerprint.current) {
          lastFingerprint.current = fp;
          setRuns(out);
        }
        setError(null);
        setLoading(false);
      } catch (e) {
        if (!cancelled) {
          setError((e as Error).message);
          setLoading(false);
        }
      }
      if (!cancelled) {
        // Skip the poll while the tab is hidden — browsers throttle
        // setTimeout but the burst of stale requests on focus-back
        // still hammered the server. Re-arm via visibilitychange.
        const delay =
          typeof document !== "undefined" && document.hidden
            ? Math.max(pollMsRef.current * 4, 15_000)
            : pollMsRef.current;
        timer = window.setTimeout(fetchRuns, delay);
      }
    };
    fetchRuns();
    const onVisibility = () => {
      if (!document.hidden && !cancelled) {
        if (timer != null) {
          clearTimeout(timer);
          timer = null;
        }
        fetchRuns();
      }
    };
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", onVisibility);
    }
    return () => {
      cancelled = true;
      if (timer != null) clearTimeout(timer);
      if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", onVisibility);
      }
    };
  }, [status, limit]);

  return { runs, counts, loading, error };
}
