import { ChevronLeftIcon, ChevronUpIcon } from "@radix-ui/react-icons";
import { Separator } from "react-resizable-panels";

import { IconButton } from "@/components/ui";

// Collapsed-panel re-open affordance: the thin strip that replaces the
// detail panel (right) or event-log drawer (bottom) when collapsed.
export function ExpandStrip({
  orientation,
  label,
  onClick,
}: {
  orientation: "right" | "bottom";
  label: string;
  onClick: () => void;
}) {
  const isRight = orientation === "right";
  const stripClass = isRight
    ? "flex flex-col items-center justify-start border-l w-7 py-2"
    : "flex items-center justify-center border-t h-7";
  return (
    <div
      className={`${stripClass} border-border-default bg-surface-1 shrink-0 animate-fade-in-opacity`}
    >
      <IconButton label={label} size="sm" variant="ghost" onClick={onClick}>
        {isRight ? <ChevronLeftIcon /> : <ChevronUpIcon />}
      </IconButton>
    </div>
  );
}

export function ResizeSeparator({
  orientation,
}: {
  orientation: "horizontal" | "vertical";
}) {
  // The Group's orientation defines the layout axis; the visible
  // separator runs perpendicular to it. A horizontal Group lays out
  // panels left-to-right, so the separator is a vertical bar (1px
  // wide); a vertical Group stacks top-to-bottom, so it's a horizontal
  // bar (1px tall).
  const isHorizontalGroup = orientation === "horizontal";
  return (
    <Separator
      className={
        isHorizontalGroup
          ? "w-1 bg-border-default/40 hover:bg-accent transition-colors data-[separator-state=drag]:bg-accent"
          : "h-1 bg-border-default/40 hover:bg-accent transition-colors data-[separator-state=drag]:bg-accent"
      }
      aria-label={isHorizontalGroup ? "Resize detail panel" : "Resize event log"}
    />
  );
}
