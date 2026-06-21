import { memo } from "react";

import type { RunSummary } from "@/api/runs";
import { Badge } from "@/components/ui/Badge";
import { LiveDot } from "@/components/ui/LiveDot";
import { formatRelative } from "@/lib/format";

import { STATUS_VARIANT, labelForStatus } from "../runStatusMeta";

import { BotAvatar } from "./BotAvatar";
import {
  formatDuration,
  friendlyLabel,
  shortRunID,
  workflowDisplay,
} from "./runListFormat";
import { SourceBadge } from "./SourceBadge";

export const RunListCard = memo(function RunListCard({
  run,
  onOpen,
  onFilterBot,
}: {
  run: RunSummary;
  onOpen: (id: string) => void;
  onFilterBot: (botKey: string) => void;
}) {
  // A div (not a <button>) so the bot-avatar button can nest validly;
  // keyboard semantics are restored via role + Enter/Space handling.
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onOpen(run.id)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen(run.id);
        }
      }}
      className="w-full text-left px-4 py-3 flex flex-col gap-1 min-h-[44px] cursor-pointer hover:bg-surface-2 active:bg-surface-3"
    >
      <div className="flex items-center gap-2 min-w-0 flex-wrap">
        <BotAvatar run={run} onFilter={onFilterBot} />
        <Badge variant={STATUS_VARIANT[run.status]}>
          {labelForStatus(run.status)}
        </Badge>
        <SourceBadge run={run} />
        {run.active && (
          <LiveDot tone="live" size="sm" label="Active in this process" />
        )}
        <span className="font-medium truncate">
          {friendlyLabel(run)}
        </span>
      </div>
      {workflowDisplay(run) && (
        <div className="text-micro text-fg-default truncate">
          {workflowDisplay(run)}
        </div>
      )}
      <div className="text-micro text-fg-muted flex flex-wrap gap-x-2">
        <span>{formatRelative(run.created_at)}</span>
        <span>·</span>
        <span>{formatDuration(run.created_at, run.finished_at)}</span>
      </div>
      <div
        className="text-caption text-fg-subtle font-mono truncate"
        title={run.id}
      >
        {shortRunID(run.id)}
      </div>
    </div>
  );
});
