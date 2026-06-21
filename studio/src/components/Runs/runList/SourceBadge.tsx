import { memo } from "react";

import type { RunSummary } from "@/api/runs";
import { Badge } from "@/components/ui/Badge";

import { metaForSource, runSourceKind } from "../runSourceMeta";

// SourceBadge renders the derived source classification for a run.
// Empty / unknown source_kind values normalise to "manual" so legacy
// rows still get a glyph instead of an awkward blank cell.
export const SourceBadge = memo(function SourceBadge({ run }: { run: RunSummary }) {
  const kind = runSourceKind(run);
  const meta = metaForSource(kind);
  const Icon = meta.Icon;
  return (
    <Badge
      variant={meta.variant}
      size="sm"
      title={meta.description}
      leadingIcon={<Icon className="w-3 h-3" />}
    >
      {meta.label}
    </Badge>
  );
});
