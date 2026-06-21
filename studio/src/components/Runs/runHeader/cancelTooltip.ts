// Extracted from RunHeader.tsx to keep that file focused.
// Per-status hover text for the RunHeader Cancel button.

import type { RunHeader as RunHeaderType } from "@/api/runs";

// cancelTooltip tailors the Cancel button's hover text to the run's
// status so operators understand the difference between aborting an
// in-flight run, giving up on a human gate, and dropping a queued run
// before any runner sees it.
// Exported for the unit test that locks the per-status wording.
export function cancelTooltip(status: RunHeaderType["status"]): string {
  switch (status) {
    case "queued":
      return "Drop from the queue before any runner picks this up.";
    case "paused_waiting_human":
    case "paused_operator":
      return "Cancel without answering — the run terminates.";
    case "running":
      return "Stop the run as soon as the engine reaches a safe boundary.";
    default:
      return "Cancel this run.";
  }
}
