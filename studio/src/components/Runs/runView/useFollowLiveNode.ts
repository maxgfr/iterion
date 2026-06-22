import { useEffect, useMemo, useState } from "react";

import {
  type ExecutionState,
  type RunEvent,
  type RunStatus,
} from "@/api/runs";
import { buildExecutionsAt } from "@/lib/snapshotReducer";
import { selectRunningExecution } from "@/store/run";

// Live-follow node id with sticky fallback — lifted out of RunView.
//
// Sticky follow-live cache: bridges the transient gap between
// `node_finished` (previous exec flips to finished) and `node_started`
// (next exec arrives). Those events are emitted by the engine across
// separate WS messages — with a `SaveCheckpoint` disk I/O + edge
// selection in between — so the client sees a brief window where no
// execution carries status="running" even though the run is still
// active and producing logs.
//
// Without this cache, `runningExec` flips to null during the window,
// the parent's wfSelectedNodeId collapses to manualSelectedNodeId
// (null when follow-live is engaged), and the detail panel + per-node
// log filter both blank out until the next node_started lands. With
// it, we hold the last known running node id so the UI stays anchored
// through the gap and only updates once the new running exec
// materialises.
//
// followLiveNodeId resolution:
//   - When scrubbing/replaying, derive the running node from the
//     historical exec map at scrubSeq so the canvas focus advances
//     with the timeline instead of staying stuck on the user's last
//     manual pick.
//   - When live and `runningExec` is non-null we always use its node id
//     (truth).
//   - When `runningExec` is null but the run is still active, we fall
//     back to `lastRunningNodeId` — typically the just-finished node —
//     so the follow-live UI doesn't blank out mid-transition.
//   - When the run reached a terminal state, return null and clear the
//     cache so we don't keep showing a stale "live" node after finish/
//     fail/cancel. Paused intentionally keeps the cached node — the
//     user is mid-interaction.
export interface FollowLiveNode {
  followLiveNodeId: string | null;
  runningExec: ExecutionState | null;
}

export function useFollowLiveNode({
  runId,
  scrubSeq,
  events,
  executionsById,
  runStatus,
}: {
  runId: string | null;
  scrubSeq: number | null;
  events: RunEvent[];
  executionsById: Map<string, ExecutionState>;
  runStatus: RunStatus | undefined;
}): FollowLiveNode {
  // When follow-live is on, override the manual pick with the
  // currently-running execution. While scrubbing the timeline we
  // disable the auto-track so the panel reflects the past, not the
  // live tail.
  const runningExec = useMemo(() => {
    if (scrubSeq !== null) return null;
    return selectRunningExecution(executionsById);
  }, [scrubSeq, executionsById]);

  const [lastRunningNodeId, setLastRunningNodeId] = useState<string | null>(
    null,
  );
  useEffect(() => {
    if (runningExec) {
      setLastRunningNodeId(runningExec.ir_node_id);
    }
  }, [runningExec]);
  // Clear the cache when the run reaches a terminal state so we don't
  // keep showing a stale "live" node after finish/fail/cancel. Paused
  // intentionally keeps the cached node — the user is mid-interaction.
  useEffect(() => {
    if (
      runStatus === "finished" ||
      runStatus === "failed" ||
      runStatus === "failed_resumable" ||
      runStatus === "cancelled"
    ) {
      setLastRunningNodeId(null);
    }
  }, [runStatus]);
  // Reset the cache on run change so the previous run's last-running
  // node doesn't carry over.
  useEffect(() => {
    setLastRunningNodeId(null);
  }, [runId]);

  const followLiveNodeId = useMemo(() => {
    if (scrubSeq !== null) {
      const execs = buildExecutionsAt(events, scrubSeq);
      let best: ExecutionState | null = null;
      for (const e of execs) {
        if (e.status !== "running") continue;
        if (!best || (e.started_at ?? "") > (best.started_at ?? "")) {
          best = e;
        }
      }
      return best?.ir_node_id ?? null;
    }
    if (runningExec) return runningExec.ir_node_id;
    if (runStatus === "running" || runStatus === "paused_waiting_human") {
      return lastRunningNodeId;
    }
    return null;
  }, [runningExec, scrubSeq, events, runStatus, lastRunningNodeId]);

  return { followLiveNodeId, runningExec };
}
