// Native-tracker board state palette. Single source of truth for any
// view that renders a state badge — keeps the Board columns, the
// WatchPanel chips, and any future state-aware surface in lockstep
// with the CSS vars declared in app.css.

import type React from "react";

export const TERMINAL_BOARD_STATES: ReadonlySet<string> = new Set([
  "done",
  "blocked",
  "cancelled",
]);

// stateCssVar returns the CSS variable reference for a state's
// canonical color. Unknown states fall back to the operator-tuneable
// (eligible? ready : terminal? done : backlog) decision used by the
// Board view's column header.
export function stateCssVar(
  name: string,
  opts: { eligible?: boolean; terminal?: boolean } = {},
): string {
  switch (name) {
    case "backlog":
      return "var(--color-board-backlog)";
    case "ready":
      return "var(--color-board-ready)";
    case "in_progress":
      return "var(--color-board-in-progress)";
    case "review":
      return "var(--color-board-review)";
    case "done":
      return "var(--color-board-done)";
    case "blocked":
    case "failed":
    case "cancelled":
      return "var(--color-board-blocked)";
    default:
      if (opts.terminal) return "var(--color-board-done)";
      if (opts.eligible) return "var(--color-board-ready)";
      return "var(--color-board-backlog)";
  }
}

// stateChipStyle returns the inline style for a compact state chip —
// 15% backdrop over the canonical color so the chip reads tinted
// without dominating the row. color-mix keeps the relation correct
// across light/dark themes (the var flips, the alpha doesn't).
export function stateChipStyle(name: string): React.CSSProperties {
  const c = stateCssVar(name);
  return {
    color: c,
    backgroundColor: `color-mix(in oklch, ${c} 15%, transparent)`,
  };
}
