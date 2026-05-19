// isTypingTarget returns true when the keyboard event's target is an
// editable surface — input, textarea, select, or any contenteditable
// element. Shared by every window-scoped keyboard handler so a single
// rule keeps text fields immune from app-wide shortcuts.
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (target.isContentEditable) return true;
  return false;
}

// isMacOS returns true when the UA suggests a Mac. Used to pick the
// glyph for keyboard-shortcut hints (⌘ vs Ctrl). Safe in SSR — falls
// back to false when navigator is unavailable.
export function isMacOS(): boolean {
  return (
    typeof navigator !== "undefined" &&
    navigator.userAgent.toLowerCase().includes("mac")
  );
}
