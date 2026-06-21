import { useEffect, useMemo, useState } from "react";

import type { RunSummary } from "@/api/runs";

export interface UseRunListLiveTickResult {
  // Captured `Date.now()` anchored on the latest tick. Sort and
  // duration consumers should derive from `now` so re-renders within
  // the same tick produce a stable order.
  now: number;
}

// Force a re-render once per second while at least one visible run
// is still in-flight (no finished_at), so the duration column ticks
// forward instead of freezing on whatever value the last poll
// produced. Idle when every visible run has finished.
export function useRunListLiveTick(runs: RunSummary[]): UseRunListLiveTickResult {
  const hasLiveRun = useMemo(() => runs.some((r) => !r.finished_at), [runs]);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!hasLiveRun) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [hasLiveRun]);
  // Anchor `now` on the captured tick so derived sort orders stay
  // stable across re-renders within the same second.
  const now = useMemo(() => Date.now(), [tick]);
  return { now };
}
