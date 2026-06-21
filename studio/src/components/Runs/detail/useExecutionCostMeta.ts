import { useMemo } from "react";

import type { RunEvent } from "@/api/runs";
import { readNodeOutputMeta } from "@/lib/delegateMeta";

export interface ExecutionCostMeta {
  costUsd: number;
  tokens: number;
  model: string;
  contextWindow: number;
  contextUsed: number;
  thinkingTokens: number;
  thinkingMs: number;
}

// Per-execution cost + tokens are sourced from the node_finished
// event the runtime emits with the cost.Annotate output. A single
// execution emits at most one node_finished, so summing across the
// matching events is just to defensively merge if the engine ever
// emits multiple (e.g. on retry within a node) — the common case is
// exactly one row.
export function useExecutionCostMeta(events: RunEvent[]): ExecutionCostMeta {
  return useMemo(() => {
    let costUsd = 0;
    let tokens = 0;
    let model = "";
    let contextWindow = 0;
    let contextUsed = 0;
    let thinkingTokens = 0;
    let thinkingMs = 0;
    for (const e of events) {
      if (e.type !== "node_finished" || !e.data) continue;
      const c = e.data["_cost_usd"];
      if (typeof c === "number") costUsd += c;
      const t = e.data["_tokens"];
      if (typeof t === "number") tokens += t;
      const meta = readNodeOutputMeta(
        e.data["output"] as Record<string, unknown> | undefined,
      );
      if (meta.model && !model) model = meta.model;
      if (meta.contextWindow && meta.contextWindow > contextWindow)
        contextWindow = meta.contextWindow;
      if (meta.contextUsed && meta.contextUsed > contextUsed)
        contextUsed = meta.contextUsed;
      if (meta.thinkingTokens) thinkingTokens += meta.thinkingTokens;
      if (meta.thinkingMs) thinkingMs += meta.thinkingMs;
    }
    return {
      costUsd,
      tokens,
      model,
      contextWindow,
      contextUsed,
      thinkingTokens,
      thinkingMs,
    };
  }, [events]);
}
