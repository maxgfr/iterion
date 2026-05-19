import { useUIStore } from "@/store/ui";

// Slim header bar above <main>. Only renders when a page has pushed
// `left` and/or `right` content via useHeaderSlot. Pages without any
// header content skip this entirely, so the main content area sits
// flush against the top of the viewport.
//
// Also hosts the in-page anchor that the "Skip to main content"
// keyboard-only link in AppShell targets.
export default function ContextualHeaderBar() {
  const left = useUIStore((s) => s.headerLeft);
  const right = useUIStore((s) => s.headerRight);

  if (!left && !right) return null;

  return (
    <div className="shrink-0 h-10 flex items-center gap-3 px-3 sm:px-4 text-sm bg-surface-1 border-b border-border-default overflow-hidden">
      <div className="flex items-center gap-2 min-w-0 flex-1">{left}</div>
      {right && (
        <div className="flex items-center gap-1.5 sm:gap-2 shrink-0">{right}</div>
      )}
    </div>
  );
}
