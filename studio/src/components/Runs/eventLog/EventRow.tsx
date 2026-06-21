import { memo, useCallback } from "react";

import { EVENT_BADGE, indentForType, type AnnotatedEvent } from "./eventModel";

// Memoised so Virtuoso can skip re-rendering the ~20 visible rows when
// only ancillary parent state changes (followTail toggle, filter chip
// hover, etc.). The annotated event cache upstream preserves identity
// for unchanged events, and `onSelectNodeIteration` is useCallback'd by
// RunView — so the memo actually hits on subsequent log chunks.
export const EventRow = memo(function EventRow({
  ann,
  onSelectNodeIteration,
}: {
  ann: AnnotatedEvent;
  onSelectNodeIteration?: (nodeId: string, index: number) => void;
}) {
  const e = ann.event;
  const badge = EVENT_BADGE[e.type] ?? "bg-surface-2 text-fg-muted";
  const indent = indentForType(e.type);
  const handleClick = useCallback(() => {
    if (e.node_id && onSelectNodeIteration && ann.executionIndex >= 0) {
      onSelectNodeIteration(e.node_id, ann.executionIndex);
    }
  }, [e.node_id, ann.executionIndex, onSelectNodeIteration]);
  return (
    <button
      type="button"
      onClick={handleClick}
      className="w-full grid grid-cols-[auto_auto_auto_1fr] gap-2 py-0.5 text-left font-mono text-caption hover:bg-surface-2 rounded px-1"
      title={
        e.node_id
          ? `Jump to ${e.node_id} (attempt ${ann.executionIndex + 1})`
          : undefined
      }
    >
      <span className="text-fg-subtle">{e.seq.toString().padStart(4, "0")}</span>
      <span
        className={`px-1.5 rounded ${badge}`}
        style={indent > 0 ? { marginLeft: indent * 12 } : undefined}
      >
        {indent > 0 && (
          <span className="text-fg-subtle mr-1" aria-hidden="true">
            ↳
          </span>
        )}
        {e.type}
      </span>
      <span className="text-fg-default truncate">{e.node_id ?? "-"}</span>
      <span className="text-fg-subtle truncate">{ann.preview}</span>
    </button>
  );
});
