import { type Dispatch, type SetStateAction, useEffect } from "react";
import { useReactFlow, type Node as FlowNode } from "@xyflow/react";

// Centre on the selected node when selection changes (jump-to-failed,
// running node advances) AND when the layout itself settles (initial
// mount: parent's snapshot can populate selectedNodeId BEFORE the IR
// fetch + ELK layout produce `nodes`, so depending on selectedNodeId
// alone leaves us silently exited with nodes=[]). `layoutEpoch` bumps
// exactly once per autoLayout completion, so per-event nodes patches
// (executions advancing, iteration changes) don't re-fire setCenter.
//
// Also pulses the freshly-selected node for ~600ms via a transient
// className so the user sees the jump even when the canvas is already
// showing the target — a common case after clicking an EventLog row
// whose node is the current viewport's centre. The pulse is purely
// additive (transient class on the FlowNode wrapper) and clears itself
// so ref-counted highlight state stays absent from the React tree.
export function useSelectedNodeFocus({
  selectedNodeId,
  layoutEpoch,
  nodes,
  setNodes,
}: {
  selectedNodeId: string | null;
  layoutEpoch: number;
  nodes: FlowNode[];
  setNodes: Dispatch<SetStateAction<FlowNode[]>>;
}) {
  const reactFlow = useReactFlow();
  useEffect(() => {
    if (!selectedNodeId) return;
    const node = nodes.find((n) => n.id === selectedNodeId);
    if (!node) return;
    reactFlow.setCenter(
      node.position.x + 100,
      node.position.y + 40,
      { zoom: 1, duration: 350 },
    );
    setNodes((prev) =>
      prev.map((n) =>
        n.id === selectedNodeId
          ? { ...n, className: `${n.className ?? ""} pulse-flash` }
          : n,
      ),
    );
    const t = setTimeout(() => {
      setNodes((prev) =>
        prev.map((n) =>
          n.id === selectedNodeId
            ? {
                ...n,
                className: (n.className ?? "").replace(" pulse-flash", "").trim(),
              }
            : n,
        ),
      );
    }, 600);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedNodeId, layoutEpoch]);
}
