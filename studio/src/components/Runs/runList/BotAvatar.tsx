import { memo } from "react";

import type { RunSummary } from "@/api/runs";

import { botEmoji, botLabel } from "../runBotMeta";
import { workflowLabel } from "../runListSortGroup";

// BotAvatar is the per-row bot glyph that doubles as a quick filter:
// clicking it filters the list to that bot (toggling off if already
// active). stopPropagation keeps the row's open-run click from firing.
export const BotAvatar = memo(function BotAvatar({
  run,
  onFilter,
}: {
  run: RunSummary;
  onFilter: (botKey: string) => void;
}) {
  const key = workflowLabel(run);
  if (!key) return null;
  const label = botLabel(run);
  return (
    <button
      type="button"
      title={`Filter by ${label}`}
      aria-label={`Filter by ${label}`}
      onClick={(e) => {
        e.stopPropagation();
        onFilter(key);
      }}
      className="shrink-0 inline-flex items-center justify-center w-5 h-5 rounded text-sm leading-none hover:bg-surface-3"
    >
      <span aria-hidden>{botEmoji(run)}</span>
    </button>
  );
});
