import { Cross2Icon, PlusIcon } from "@radix-ui/react-icons";
import type { ReactNode } from "react";

import type { Tab } from "@/store/tabs";

interface Props {
  tabs: Tab[];
  activeTabId: string | null;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onNewTab?: () => void;
  newTabLabel?: string;
  icon: (tab: Tab) => ReactNode;
  emptyState?: ReactNode;
}

// InnerTabBar — horizontal strip rendered at the top of the editor /
// runs sections. Lives inside the section's view (not at AppShell
// level) so the global navigation chrome stays uncluttered. Shared
// between EditorTabsView and RunsTabsView; the per-section view picks
// the kind-specific icon + new-tab callback.
export default function InnerTabBar({
  tabs,
  activeTabId,
  onSelect,
  onClose,
  onNewTab,
  newTabLabel,
  icon,
  emptyState,
}: Props) {
  if (tabs.length === 0 && emptyState) {
    return (
      <div className="shrink-0 h-9 flex items-center gap-3 px-3 text-xs text-fg-subtle bg-surface-1 border-b border-border-default">
        {emptyState}
      </div>
    );
  }
  return (
    <div
      className="shrink-0 flex items-stretch gap-px h-9 bg-surface-1 border-b border-border-default overflow-x-auto"
      role="tablist"
    >
      {tabs.map((tab) => {
        const active = tab.id === activeTabId;
        const stateCls = active
          ? "bg-surface-0 text-fg-default border-t-2 border-t-accent"
          : "bg-surface-1 text-fg-muted hover:bg-surface-2 hover:text-fg-default border-t-2 border-t-transparent";
        return (
          <div
            key={tab.id}
            role="tab"
            aria-selected={active}
            className={`shrink-0 inline-flex items-center gap-1.5 pl-2.5 pr-1 max-w-[200px] text-xs border-r border-border-default group ${stateCls}`}
            title={tab.label}
          >
            <button
              type="button"
              onClick={() => onSelect(tab.id)}
              className="inline-flex items-center gap-1.5 min-w-0 py-1 focus:outline-none"
            >
              {icon(tab)}
              <span className="truncate">{tab.label}</span>
            </button>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onClose(tab.id);
              }}
              className="inline-flex items-center justify-center w-5 h-5 rounded text-fg-subtle hover:text-fg-default hover:bg-surface-3 opacity-0 group-hover:opacity-100 focus:opacity-100"
              title="Close tab"
              aria-label={`Close tab ${tab.label}`}
            >
              <Cross2Icon className="w-3 h-3" />
            </button>
          </div>
        );
      })}
      {onNewTab && (
        <button
          type="button"
          onClick={onNewTab}
          className="shrink-0 inline-flex items-center justify-center h-full w-9 text-fg-subtle hover:text-fg-default hover:bg-surface-2"
          title={newTabLabel ?? "New tab"}
          aria-label={newTabLabel ?? "New tab"}
        >
          <PlusIcon className="w-3.5 h-3.5" />
        </button>
      )}
    </div>
  );
}
