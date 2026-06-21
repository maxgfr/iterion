import { useMemo } from "react";

import type { ExecutionState, RunEvent } from "@/api/runs";
import { stepIteration } from "@/lib/eventIter";

export function useExecutionEvents(events: RunEvent[], exec: ExecutionState | null) {
  return useMemo<RunEvent[]>(() => {
    if (!exec) return [];
    const out: RunEvent[] = [];
    const counts = new Map<string, number>();
    for (const e of events) {
      if (!e.node_id) continue;
      const iter = stepIteration(counts, e);
      if (
        (e.branch_id || "main") === exec.branch_id &&
        e.node_id === exec.ir_node_id &&
        iter === exec.loop_iteration
      ) {
        out.push(e);
      }
    }
    return out;
  }, [events, exec]);
}
