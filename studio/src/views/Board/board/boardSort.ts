import type { NativeIssue } from "@/api/native";

import type { SortMode } from "../boardShared";

// Pre-dispatch lanes: an issue here has not yet entered the dispatcher's
// eligible queue, so it can be "dispatched" (transitioned into the
// dispatch lane). review/done/blocked are downstream and not dispatchable.
export function isDispatchable(state: string): boolean {
  return state === "inbox" || state === "backlog";
}

// Above this many at once, bulk dispatch asks for confirmation — each
// dispatch starts a paid run.
export const BULK_DISPATCH_CONFIRM_THRESHOLD = 3;

// sortComparator returns the per-column ordering for a sort mode. Priority
// is descending (higher number first, the board's long-standing default);
// date modes are newest-first; title is alphabetical.
export function sortComparator(
  mode: SortMode,
): (a: NativeIssue, b: NativeIssue) => number {
  switch (mode) {
    case "updated":
      return (a, b) => (b.updated_at ?? "").localeCompare(a.updated_at ?? "");
    case "created":
      return (a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? "");
    case "title":
      return (a, b) => (a.title ?? "").localeCompare(b.title ?? "");
    case "priority":
    default:
      return (a, b) => (b.priority ?? 0) - (a.priority ?? 0);
  }
}
