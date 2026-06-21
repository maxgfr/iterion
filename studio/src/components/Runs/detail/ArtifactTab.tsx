import type { ArtifactSummary } from "@/api/runs";

import ArtifactDiff from "../ArtifactDiff";

export function ArtifactTab({
  runId,
  nodeId,
  versions,
}: {
  runId: string;
  nodeId: string;
  versions: ArtifactSummary[];
}) {
  const hasArtifact = versions.length > 0;
  return (
    <div className="overflow-auto px-4 py-3 h-full">
      {!hasArtifact ? (
        <div className="text-fg-subtle">No artifact published.</div>
      ) : (
        <ArtifactDiff runId={runId} nodeId={nodeId} versions={versions} />
      )}
    </div>
  );
}
