import { Dialog } from "@/components/ui/Dialog";
import { isMacOS } from "@/lib/keyboard";

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function ShortcutsHelp({ open, onClose }: Props) {
  // The modifier glyph differs by platform — macOS users expect ⌘
  // (Command), every other platform sees "Ctrl". Computed inside the
  // component body (not at module scope) so SSR / test environments
  // where `navigator` is unavailable still resolve to a sensible
  // default at mount time.
  const mod = isMacOS() ? "⌘" : "Ctrl";
  const shortcuts = [
    { keys: `${mod}+Z`, desc: "Undo" },
    { keys: `${mod}+Y / ${mod}+Shift+Z`, desc: "Redo" },
    { keys: `${mod}+S`, desc: "Save" },
    { keys: `${mod}+C`, desc: "Copy selected node" },
    { keys: `${mod}+V`, desc: "Paste copied node" },
    { keys: "Delete / Backspace", desc: "Delete selected node or edge" },
    { keys: "Right-click node", desc: "Context menu (set entry, duplicate, delete)" },
    { keys: "Drag from handle", desc: "Quick-add node with auto-connect" },
    { keys: "/", desc: "Search nodes" },
    { keys: "Escape", desc: "Clear selection / close dialogs" },
    { keys: "?", desc: "Show this help" },
  ];
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
      title="Keyboard Shortcuts"
      widthClass="max-w-md"
    >
      <div className="px-4 py-3 space-y-2">
        {shortcuts.map(({ keys, desc }) => (
          <div key={keys} className="flex items-center justify-between gap-4">
            <span className="text-xs text-fg-muted">{desc}</span>
            <kbd className="bg-surface-2 border border-border-strong rounded px-2 py-0.5 text-caption text-fg-default font-mono whitespace-nowrap">
              {keys}
            </kbd>
          </div>
        ))}
        <p className="pt-3 mt-3 border-t border-border-default text-caption text-fg-subtle text-center">
          Press Escape to close
        </p>
      </div>
    </Dialog>
  );
}
