import { useCallback, useEffect, useRef } from "react";
import type { VirtuosoHandle } from "react-virtuoso";

// Slack the bottom-detection threshold so dynamic-height row reflows
// don't transiently report "not at bottom" while followOutput re-aligns.
export const AT_BOTTOM_THRESHOLD_PX = 48;

export interface UseVirtuosoTailFollowParams {
  followTail: boolean;
  filteredLength: number;
  errorIndices: number[];
  onToggleFollow: (next: boolean) => void;
}

export interface UseVirtuosoTailFollowResult {
  virtuosoRef: React.RefObject<VirtuosoHandle | null>;
  handleToggleFollow: (next: boolean) => void;
  jumpToNextError: () => void;
  handleIsScrolling: (s: boolean) => void;
  handleAtBottomStateChange: (atBottom: boolean) => void;
}

// Owns Virtuoso's tail-follow + jump-to-error coordination. Splits out
// the explicit tail effect (Virtuoso's `followOutput="auto"` isn't
// reliable on live runs where dynamic-height measurement transiently
// reports not-at-bottom mid-batch), the manual-vs-scroll disengage
// memory, and the error-cycle cursor.
export function useVirtuosoTailFollow({
  followTail,
  filteredLength,
  errorIndices,
  onToggleFollow,
}: UseVirtuosoTailFollowParams): UseVirtuosoTailFollowResult {
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  // Tracks whether virtuoso is currently scrolling. atBottomStateChange
  // also fires on data/filter changes; we only treat "left the bottom"
  // as a user intent to disable follow-tail when an actual scroll is in
  // flight.
  const isScrollingRef = useRef<boolean>(false);
  // True when the most recent follow-tail disable came from a scroll-up
  // (vs the checkbox). Lets us auto-re-engage when the user scrolls back
  // to the tail, while keeping a manual uncheck sticky.
  const disabledByScrollRef = useRef<boolean>(false);

  // Cycle through error events on repeated clicks of the "n errors"
  // badge: scroll to the first one, then the next, etc. — wraps around
  // at the end. The cursor sits in a ref so the parent doesn't
  // re-render between clicks.
  const errorCursorRef = useRef<number>(-1);
  const jumpToNextError = useCallback(() => {
    if (errorIndices.length === 0) return;
    errorCursorRef.current = (errorCursorRef.current + 1) % errorIndices.length;
    const target = errorIndices[errorCursorRef.current]!;
    virtuosoRef.current?.scrollToIndex({
      index: target,
      align: "center",
      behavior: "smooth",
    });
    // Disengage tail-follow so the auto-scroll doesn't immediately
    // yank the user back to live.
    if (followTail) onToggleFollow(false);
  }, [errorIndices, followTail, onToggleFollow]);

  // Virtuoso's `followOutput="auto"` only fires when it considers the
  // user "at bottom", which is unreliable on a live run where events
  // arrive across multiple micro-tasks: between batches, dynamic-height
  // measurement can briefly report not-at-bottom and skip the scroll.
  // Drive the scroll explicitly from `filtered.length` while followTail
  // is on; the `atBottomStateChange` disengage below still flips the
  // toggle off when the user scrolls up.
  useEffect(() => {
    if (followTail && filteredLength > 0) {
      virtuosoRef.current?.scrollToIndex({
        index: filteredLength - 1,
        align: "end",
        behavior: "auto",
      });
    }
  }, [followTail, filteredLength]);

  const handleToggleFollow = useCallback(
    (next: boolean) => {
      // A direct checkbox interaction is always treated as manual intent,
      // overriding any prior "disabled by scroll" memory.
      disabledByScrollRef.current = false;
      onToggleFollow(next);
      // Re-engaging the toggle while scrolled up shouldn't wait for the
      // next event to arrive — jump to the tail immediately.
      if (next && filteredLength > 0) {
        virtuosoRef.current?.scrollToIndex({
          index: filteredLength - 1,
          align: "end",
          behavior: "auto",
        });
      }
    },
    [onToggleFollow, filteredLength],
  );

  const handleIsScrolling = useCallback((s: boolean) => {
    isScrollingRef.current = s;
  }, []);

  const handleAtBottomStateChange = useCallback(
    (atBottom: boolean) => {
      if (!atBottom && followTail && isScrollingRef.current) {
        disabledByScrollRef.current = true;
        onToggleFollow(false);
      } else if (
        atBottom &&
        !followTail &&
        disabledByScrollRef.current
      ) {
        // No isScrolling guard: atBottomStateChange(true) can race
        // with isScrolling(false) on momentum scrolls, leaving the
        // ref already cleared. disabledByScrollRef alone is enough
        // to distinguish scroll-disabled from manually-disabled.
        disabledByScrollRef.current = false;
        onToggleFollow(true);
      }
    },
    [followTail, onToggleFollow],
  );

  return {
    virtuosoRef,
    handleToggleFollow,
    jumpToNextError,
    handleIsScrolling,
    handleAtBottomStateChange,
  };
}
