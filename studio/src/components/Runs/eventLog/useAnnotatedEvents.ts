import { useMemo, useRef } from "react";

import type { RunEvent } from "@/api/runs";
import { stepIteration } from "@/lib/eventIter";

import { previewData, type AnnotatedEvent } from "./eventModel";

interface AnnotatedCache {
  events: RunEvent[];
  annotated: AnnotatedEvent[];
  counts: Map<string, number>;
  typeCounts: Map<string, number>;
  // Per-(branch, node) counters and last-seen exec_id, paralleling
  // the live store's lastExecIDByNode. Threading these across cache
  // invocations is what makes the annotation pass O(K) on the new
  // tail rather than O(N) on every batch flush.
  execIndexCounts: Map<string, number>;
  lastExecID: Map<string, string>;
}

export interface UseAnnotatedEventsResult {
  annotated: AnnotatedEvent[];
  typeCounts: Map<string, number>;
}

// Walks `events` and produces an annotated mirror plus per-type counts.
// Maintains an incremental cache: when `events` only grows by appending
// at the tail (the common live case once history is replayed), reuses
// the previously-annotated prefix and only annotates the new tail.
// Falls back to a full recompute when the array prefix changes
// (snapshot replay, MAX_EVENTS eviction, runId switch). This drops
// per-event cost from O(N) to O(K) where K is the number of new events.
export function useAnnotatedEvents(events: RunEvent[]): UseAnnotatedEventsResult {
  const cacheRef = useRef<AnnotatedCache | null>(null);

  return useMemo(() => {
    const cache = cacheRef.current;
    let baseAnnotated: AnnotatedEvent[];
    let counts: Map<string, number>;
    let typeCountsMap: Map<string, number>;
    let execIndexCounts: Map<string, number>;
    let lastExecID: Map<string, string>;
    let startIdx = 0;

    // Reuse the cache when the live events array has only grown at the
    // tail. A snapshot replay produces a fresh `events` array whose
    // RunEvent objects are deserialized from scratch — the reference
    // check at index 0 detects that boundary and forces a full
    // recompute. The cached array is mutated in place below; nothing
    // outside this useMemo retains it across renders.
    const cachedLen = cache?.annotated.length ?? 0;
    const reusable =
      cache !== null &&
      cachedLen > 0 &&
      cachedLen <= events.length &&
      cache.annotated[0]!.event === events[0] &&
      cache.annotated[cachedLen - 1]!.event === events[cachedLen - 1];

    if (reusable) {
      // Start a fresh array so consumers (useMemo dep, downstream
      // filter passes) see a new reference when events grow — the
      // previous in-place push kept the same array identity and
      // relied on the events dep change as a fence, which is fragile
      // if any caller ever mutated the source events array in place.
      baseAnnotated = cache.annotated.slice();
      counts = new Map(cache.counts);
      typeCountsMap = new Map(cache.typeCounts);
      execIndexCounts = new Map(cache.execIndexCounts);
      lastExecID = new Map(cache.lastExecID);
      startIdx = cachedLen;
    } else {
      baseAnnotated = [];
      counts = new Map<string, number>();
      typeCountsMap = new Map<string, number>();
      execIndexCounts = new Map<string, number>();
      lastExecID = new Map<string, string>();
    }

    for (let i = startIdx; i < events.length; i++) {
      const e = events[i]!;
      const iteration = stepIteration(counts, e);
      // Track exec_id per (branch, node) so non-node_started events
      // attribute to the right exec for the selection filter, and so
      // node_started's own count gives a stable 0-based array index.
      const branch = e.branch_id || "main";
      const key = e.node_id ? `${branch}\t${e.node_id}` : "";
      let executionId: string | null = null;
      let executionIndex = -1;
      if (e.node_id) {
        if (e.type === "node_started") {
          // New exec_id, prefer iteration_path when present (mirror of
          // the live store reducer and pkg/runview/snapshot.go).
          const rawPath = e.data?.iteration_path;
          executionId =
            typeof rawPath === "string" && rawPath.length > 0
              ? `exec:${branch}:${e.node_id}:${rawPath}`
              : `exec:${branch}:${e.node_id}:${iteration}`;
          lastExecID.set(key, executionId);
          const prevIdx = execIndexCounts.get(key);
          executionIndex = prevIdx === undefined ? 0 : prevIdx + 1;
          execIndexCounts.set(key, executionIndex);
        } else {
          executionId = lastExecID.get(key) ?? null;
          executionIndex = execIndexCounts.get(key) ?? 0;
        }
      }
      baseAnnotated.push({
        event: e,
        iteration,
        executionIndex,
        executionId,
        preview: previewData(e.data),
      });
      typeCountsMap.set(e.type, (typeCountsMap.get(e.type) ?? 0) + 1);
    }

    cacheRef.current = {
      events,
      annotated: baseAnnotated,
      counts,
      typeCounts: typeCountsMap,
      execIndexCounts,
      lastExecID,
    };
    return { annotated: baseAnnotated, typeCounts: typeCountsMap };
  }, [events]);
}
