// Helpers for reading the runtime "delegate meta" the executor stamps
// on a node's output map. The backend writes underscore-prefixed keys
// (`_model`, `_context_window`, `_context_used`) in
// pkg/backend/model/executor.go `stampDelegateOutputMeta`; the editor
// reads them in two places — the per-node runtime override map on the
// run canvas and the detail panel header — so the key names live here
// as the single mirror of the backend's wire format.

export interface DelegateOutputMeta {
  model?: string;
  reasoning_effort?: string;
  contextWindow?: number;
  contextUsed?: number;
}

export function readNodeOutputMeta(
  output: Record<string, unknown> | undefined,
): DelegateOutputMeta {
  if (!output) return {};
  const out: DelegateOutputMeta = {};
  const m = output["_model"];
  if (typeof m === "string" && m) out.model = m;
  const cw = output["_context_window"];
  if (typeof cw === "number" && cw > 0) out.contextWindow = cw;
  const cu = output["_context_used"];
  if (typeof cu === "number" && cu > 0) out.contextUsed = cu;
  return out;
}
