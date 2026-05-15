import { ThinkingFooter } from "./ThinkingFooter";
import { ToolRunningFooter } from "./ToolRunningFooter";
import { AGENTIC_TOOL_NAMES, type InFlightTool } from "../../store/run";

function isAgentic(t: InFlightTool): boolean {
  return AGENTIC_TOOL_NAMES.has(t.toolName);
}

export function ActiveFooter({
  inFlightTools,
  active,
}: {
  inFlightTools: InFlightTool[];
  active: boolean;
}) {
  // Only synchronous tools claim the footer. The list is
  // startedAt-ascending; the most recent sync tool is the tail.
  let latestSync: InFlightTool | null = null;
  for (const t of inFlightTools) {
    if (!isAgentic(t)) latestSync = t;
  }
  if (latestSync) {
    return (
      <ToolRunningFooter
        toolName={latestSync.toolName}
        startedAt={latestSync.startedAt}
      />
    );
  }
  return <ThinkingFooter active={active} />;
}
