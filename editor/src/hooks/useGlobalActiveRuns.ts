import { useEffect, useRef, useState } from "react";

import { listGlobalActiveRuns, type GlobalActiveRun } from "@/api/runs";

// Cheap fingerprint to avoid re-rendering when the list hasn't changed
// (mirrors useRuns).
function fingerprint(runs: GlobalActiveRun[]): string {
  const parts: string[] = [];
  for (const r of runs) {
    parts.push(`${r.id}:${r.status}:${r.updated_at}`);
  }
  return parts.join("|");
}

const POLL_INTERVAL_MS = 8000;

export interface UseGlobalActiveRunsResult {
  runs: GlobalActiveRun[];
  loading: boolean;
  error: string | null;
}

// Polls /api/runs/global-active so the Home view can surface runs
// active in OTHER iterion stores (other projects, the no-project
// ~/.iterion slot). Slower poll than useRuns because the inputs are
// runs the user is NOT currently watching closely; 8s keeps the
// indicator fresh without driving filesystem walks every 3s.
export function useGlobalActiveRuns(): UseGlobalActiveRunsResult {
  const [runs, setRuns] = useState<GlobalActiveRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const lastFp = useRef("");

  useEffect(() => {
    let cancelled = false;

    async function tick() {
      try {
        const next = await listGlobalActiveRuns();
        if (cancelled) return;
        const fp = fingerprint(next);
        if (fp !== lastFp.current) {
          lastFp.current = fp;
          setRuns(next);
        }
        setError(null);
      } catch (e) {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    tick();
    const handle = setInterval(tick, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(handle);
    };
  }, []);

  return { runs, loading, error };
}
