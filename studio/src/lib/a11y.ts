import type { KeyboardEvent } from "react";

// Props that promote a non-button element (a table row, list item, or card)
// to a keyboard-operable button: click + Enter/Space activation, plus the
// role/tabindex/label a screen reader needs. Spread onto the element and keep
// its own className/key/title. Replaces the ~5-line Enter/Space onKeyDown
// idiom that had drifted across a dozen clickable-row/card call-sites.
export function clickableRowProps(onActivate: () => void, label: string) {
  return {
    role: "button" as const,
    tabIndex: 0,
    "aria-label": label,
    onClick: onActivate,
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onActivate();
      }
    },
  };
}
