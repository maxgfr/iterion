import { useEffect } from "react";

export function BoardKeyboardHelp({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" || e.key === "?") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-[var(--z-modal)] bg-black/40 flex items-center justify-center"
      onClick={onClose}
    >
      <div
        className="bg-surface-1 border border-border-default rounded shadow-lg p-5 max-w-sm text-sm"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="font-semibold text-fg-default mb-3">
          Keyboard shortcuts
        </div>
        <ul className="space-y-1.5 text-fg-default">
          <ShortcutRow keys="c / n" desc="New issue" />
          <ShortcutRow keys="click" desc="Select issue" />
          <ShortcutRow keys="title / double-click" desc="Open issue" />
          <ShortcutRow keys="Ctrl/⌘+click" desc="Toggle card in selection" />
          <ShortcutRow keys="Shift+click" desc="Extend selection range" />
          <ShortcutRow keys="Ctrl/⌘+A" desc="Select all visible cards" />
          <ShortcutRow keys="x" desc="Toggle selected card" />
          <ShortcutRow keys="drag selection" desc="Move all selected cards" />
          <ShortcutRow keys="↑ ↓" desc="Navigate cards in column" />
          <ShortcutRow keys="← →" desc="Move card to previous/next column" />
          <ShortcutRow keys="Enter / e" desc="Open selected issue" />
          <ShortcutRow keys="Del / Bksp" desc="Delete selected issue" />
          <ShortcutRow keys="Esc" desc="Clear selection or close" />
        </ul>
        <button
          type="button"
          onClick={onClose}
          className="mt-4 text-xs text-fg-subtle hover:text-fg-default"
        >
          Close
        </button>
      </div>
    </div>
  );
}

function ShortcutRow({ keys, desc }: { keys: string; desc: string }) {
  return (
    <li className="flex items-center justify-between gap-4">
      <kbd className="font-mono text-xs px-1.5 py-0.5 rounded bg-surface-2 border border-border-default">
        {keys}
      </kbd>
      <span className="text-fg-muted text-xs">{desc}</span>
    </li>
  );
}
