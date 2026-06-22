import type { EffortCapabilities } from "@/api/client";
import type { ExecStatus, ExecutionState, WireNode } from "@/api/runs";
import type { DelegateOutputMeta } from "@/lib/delegateMeta";
import { effortBackendKey } from "@/hooks/useEffortCapabilities";

import type { LLMMeta } from "../IRNode";

// Status filter chip values surfaced in the canvas toolbar. Mapped onto
// ExecStatus values internally so the filter set stays small + UI-shaped
// (no "skipped" or "finished" — those don't surface a chip).
export type StatusFilter = "running" | "paused" | "failed";

// Build the meta payload IRNode uses to render the model / effort badge.
// Returns undefined when the node declares no LLM context at all and no
// runtime override has arrived yet, so the badge stays absent in the
// trivial case (compute, router, terminal nodes).
//
// Effective effort priority:
//   1. runtime override (event llm_request)
//   2. value declared in the workflow (post-expansion would only show up
//      via the override path, so a literal here is what the user wrote)
//   3. provider's documented default from the registry — flagged so
//      the badge renders attenuated.
export function buildLLMMeta(
  node: WireNode,
  override: DelegateOutputMeta | undefined,
  effortCapsByPair: Map<string, EffortCapabilities>,
): LLMMeta | undefined {
  const declared = {
    model: node.model,
    backend: node.backend,
    effort: node.reasoning_effort,
  };
  if (!declared.model && !declared.backend && !declared.effort && !override) {
    return undefined;
  }
  const activeModel = override?.model ?? declared.model;
  // Caps lookup keys off the *declared* model because that's what
  // the prefetch effect populates the cache with. Runtime events
  // sometimes log a canonical alias (e.g. event "gpt-5.5" vs workflow
  // "openai/gpt-5.5") — the registry returns identical caps for
  // both, but the cache is keyed by the literal string.
  const capsModel = declared.model ?? activeModel;
  const caps = capsModel
    ? effortCapsByPair.get(
        `${effortBackendKey(declared.backend)} ${capsModel}`,
      )
    : undefined;
  let activeEffort = override?.reasoning_effort ?? declared.effort;
  let effortIsResolvedDefault = false;
  if (!activeEffort && caps?.default) {
    activeEffort = caps.default;
    effortIsResolvedDefault = true;
  }
  return {
    model: activeModel,
    backend: declared.backend,
    reasoningEffort: activeEffort,
    runtimeOverriddenModel:
      !!override?.model && !!declared.model && override.model !== declared.model,
    runtimeOverriddenEffort:
      !!override?.reasoning_effort &&
      !!declared.effort &&
      override.reasoning_effort !== declared.effort,
    effortIsResolvedDefault,
    effortSupported: caps?.supported ?? undefined,
    contextWindow: override?.contextWindow,
    contextUsed: override?.contextUsed,
  };
}

// True when at least one of the node's executions matches one of the
// active status filters. Returns true with an empty filter set — the
// canvas should show every node when no chip is selected.
export function nodeMatchesFilters(
  execs: ExecutionState[],
  filters: Set<StatusFilter>,
): boolean {
  if (filters.size === 0) return true;
  const want: Record<StatusFilter, ExecStatus> = {
    running: "running",
    paused: "paused_waiting_human",
    failed: "failed",
  };
  for (const f of filters) {
    if (execs.some((e) => e.status === want[f])) return true;
  }
  return false;
}

// Compute the "current" iteration INDEX for an IR node — i.e. the
// 0-based position in the (start-ordered) executions array we want to
// land on when the user first opens the run console. Priority is the
// in-flight execution first, then a paused one, then the latest
// (last-started) one. Returns 0 when there are no executions yet.
//
// Index semantics — NOT scalar `loop_iteration` — because Option 3
// nested-loop exec_ids can produce multiple executions of the same node
// sharing the same scalar `loop_iteration` (e.g., the runtime's
// `currentLoopIteration` returns max() across containing loops and an
// outer loop counter can stay stuck dominating the inner counter for
// many iterations). Indexing into the start-ordered array is the only
// stable per-execution identifier the UI can key on.
export function defaultIterationFor(execs: ExecutionState[]): number {
  if (execs.length === 0) return 0;
  let runningIdx: number | undefined;
  let pausedIdx: number | undefined;
  for (let i = 0; i < execs.length; i++) {
    const e = execs[i]!;
    if (e.status === "running" && runningIdx === undefined) runningIdx = i;
    if (e.status === "paused_waiting_human" && pausedIdx === undefined) pausedIdx = i;
  }
  if (runningIdx !== undefined) return runningIdx;
  if (pausedIdx !== undefined) return pausedIdx;
  return execs.length - 1;
}
