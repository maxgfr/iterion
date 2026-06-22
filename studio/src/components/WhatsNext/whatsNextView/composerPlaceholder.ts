import type { RunStatus } from "@/api/runs";

// composerPlaceholder picks the prompt copy for the always-on
// composer based on the run state it's rendered over: a closed run
// re-seeds a fresh session, a live one folds the message into the
// running step.
export function composerPlaceholder(runStatus: RunStatus | null): string {
  if (
    runStatus === "finished" ||
    runStatus === "failed" ||
    runStatus === "cancelled"
  ) {
    return "Send a message to start a fresh Nexie session…";
  }
  return "Message Nexie — it'll fold this into the step it's running…";
}
