import { useEffect, useRef } from "react";
import { useReactFlow, type Node as FlowNode } from "@xyflow/react";

// Initial focus on arrival: once layout has settled AND a running node
// is known, frame the viewport on the running node(s) — the exact same
// call the "Center on running node" toolbar button makes. Marks done so
// subsequent running-node changes (next node starts during the run)
// don't keep re-zooming under the user. Resets on runId change so
// navigating to a different run focuses again.
export function useInitialRunningFocus({
  runId,
  layoutEpoch,
  nodes,
  runningNodeIds,
}: {
  runId: string;
  layoutEpoch: number;
  nodes: FlowNode[];
  runningNodeIds: Set<string>;
}) {
  const reactFlow = useReactFlow();
  const initialFocusDoneRef = useRef(false);
  useEffect(() => {
    initialFocusDoneRef.current = false;
  }, [runId]);
  useEffect(() => {
    if (initialFocusDoneRef.current) return;
    if (nodes.length === 0) return;
    if (runningNodeIds.size === 0) return;
    initialFocusDoneRef.current = true;
    const targets = Array.from(runningNodeIds).map((id) => ({ id }));
    // No cleanup: cancelling this rAF would defeat the whole point —
    // the patch effect's setNodes re-fires our deps within the same
    // frame, and a cleanup-based cancelAnimationFrame would clobber
    // the rAF before it runs. The `done` flag prevents re-scheduling.
    requestAnimationFrame(() => {
      reactFlow.fitView({
        nodes: targets,
        padding: 0.3,
        duration: 350,
        minZoom: 0.5,
        maxZoom: 1.5,
      });
    });
  }, [layoutEpoch, runningNodeIds, nodes, reactFlow]);
}
