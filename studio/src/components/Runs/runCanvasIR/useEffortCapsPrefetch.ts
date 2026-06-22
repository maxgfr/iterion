import { useEffect, useState } from "react";

import type { EffortCapabilities } from "@/api/client";
import type { WireWorkflow } from "@/api/runs";
import {
  effortBackendKey,
  useEffortCapabilitiesClient,
} from "@/hooks/useEffortCapabilities";

// Prefetches effort capabilities for each unique (backend, model) pair
// on the IR. Shares the React Query cache populated by AgentForm so the
// editor side panel and the run canvas don't double-fetch. Already-
// cached pairs are seeded synchronously so the bar normalises and the
// attenuated badge render on first paint; the rest update as fetches
// resolve.
//
// buildLLMMeta uses `default` to render an attenuated badge when the
// workflow declares no effort, and `supported` to normalise the bar
// fill so a model's max always renders fully.
export function useEffortCapsPrefetch(
  wf: WireWorkflow | null,
): Map<string, EffortCapabilities> {
  const effortClient = useEffortCapabilitiesClient();
  const [effortCapsByPair, setEffortCapsByPair] = useState<
    Map<string, EffortCapabilities>
  >(() => new Map());

  useEffect(() => {
    if (!wf) return;
    let cancelled = false;
    // Compute pairs OUTSIDE the state updater. React StrictMode
    // invokes the updater twice; mutating shared state (a Set) inside
    // the updater would make the second invocation skip everything
    // and commit an empty Map.
    const seen = new Set<string>();
    const seedEntries: Array<[string, EffortCapabilities]> = [];
    const toFetch: Array<{ key: string; backend: string; model: string }> = [];
    for (const n of wf.nodes) {
      if (!n.model) continue;
      const backend = effortBackendKey(n.backend);
      const key = `${backend} ${n.model}`;
      if (seen.has(key)) continue;
      seen.add(key);
      const cached = effortClient.getCached(backend, n.model);
      if (cached) seedEntries.push([key, cached]);
      else toFetch.push({ key, backend, model: n.model });
    }
    if (seedEntries.length > 0) {
      setEffortCapsByPair((prev) => {
        let mutated = false;
        const next = new Map(prev);
        for (const [key, caps] of seedEntries) {
          if (next.get(key) !== caps) {
            next.set(key, caps);
            mutated = true;
          }
        }
        return mutated ? next : prev;
      });
    }
    for (const { key, backend, model } of toFetch) {
      // Capability lookup is best-effort; on failure the canvas
      // simply renders no badge for unset effort.
      effortClient
        .fetch(backend, model)
        .then((caps) => {
          if (cancelled) return;
          setEffortCapsByPair((prev) => {
            if (prev.get(key) === caps) return prev;
            const next = new Map(prev);
            next.set(key, caps);
            return next;
          });
        })
        .catch(() => {});
    }
    return () => {
      cancelled = true;
    };
  }, [wf, effortClient]);

  return effortCapsByPair;
}
