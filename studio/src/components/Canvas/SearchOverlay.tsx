import { forwardRef } from "react";
import { Input } from "@/components/ui/Input";

interface Props {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  matchCount: number;
  currentIndex: number;
}

const SearchOverlay = forwardRef<HTMLInputElement, Props>(
  ({ searchQuery, onSearchChange, onKeyDown, matchCount, currentIndex }, ref) => {
    const hasQuery = searchQuery.trim().length > 0;

    return (
      <div className="absolute top-2 left-1/2 -translate-x-1/2 z-[var(--z-canvas)] flex items-center gap-2">
        <Input
          ref={ref}
          aria-label="Search nodes"
          className="w-64 shadow-[var(--shadow-popover)]"
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search nodes... (↑↓ navigate, Enter select)"
          autoFocus
        />
        {hasQuery && (
          <span
            aria-live="polite"
            className="text-xs text-fg-subtle bg-surface-1/90 border border-border-strong rounded px-2 py-1 whitespace-nowrap shadow-[var(--shadow-popover)]"
          >
            {matchCount > 0
              ? `${currentIndex + 1} / ${matchCount}`
              : "No matches"}
          </span>
        )}
      </div>
    );
  },
);

SearchOverlay.displayName = "SearchOverlay";

export default SearchOverlay;
