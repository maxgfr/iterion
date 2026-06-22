import { Link } from "wouter";
import {
  DoubleArrowLeftIcon,
  DoubleArrowRightIcon,
  GearIcon,
} from "@radix-ui/react-icons";

import NavLinks from "./NavLinks";
import SidebarContext from "./SidebarContext";
import UserTeamChip from "./UserTeamChip";
import { BrandWordmark } from "@/components/ui/BrandWordmark";
import { useUIStore } from "@/store/ui";

function openSettings() {
  window.dispatchEvent(new CustomEvent("iterion:open-settings"));
}

const SIDEBAR_W_EXPANDED = "w-[220px]";
const SIDEBAR_W_COLLAPSED = "w-[56px]";

// Sidebar is the persistent vertical chrome on the left of the studio.
// It hosts the iterion logo, the project + ⌘K context block, primary
// nav, and the footer with backend status / theme / user team chip.
//
// Collapse state is persisted in localStorage via the UI store. When
// collapsed (56px) every row degrades to an icon-only square button
// with native tooltips that preserve the labels.
export default function Sidebar() {
  const collapsed = useUIStore((s) => s.sidebarCollapsed);
  const toggle = useUIStore((s) => s.toggleSidebarCollapsed);

  return (
    <aside
      className={`shrink-0 ${collapsed ? SIDEBAR_W_COLLAPSED : SIDEBAR_W_EXPANDED} h-full bg-surface-1 border-r border-border-default flex flex-col transition-[width] duration-150 ease-out overflow-hidden`}
      aria-label="Primary"
    >
      {/* Logo (top). The collapse-toggle lives inline next to it when
          expanded, and at the bottom of the sidebar when collapsed —
          stacking a chevron directly under the logo in icon-only mode
          made the two squares read as a single "logo + extension" unit.
          See ToggleButton below the footer. */}
      <div
        className={`shrink-0 h-12 flex items-center ${collapsed ? "justify-center px-1.5" : "gap-2 px-3"} border-b border-border-default`}
      >
        <Link
          href="/"
          className="flex items-center gap-2 min-w-0 hover:opacity-80 transition-opacity"
          title="Iterion home"
          aria-label="Iterion home"
        >
          {/* Icon mark (restored) + wordmark. Collapsed shows the icon
              alone, centered — the wordmark's bare "I"+caret read as a
              broken glyph in the 56px rail. */}
          <img
            src="/favicon-96x96.png"
            alt=""
            aria-hidden="true"
            className="h-7 w-7 shrink-0 dark:invert"
          />
          {!collapsed && <BrandWordmark />}
        </Link>
        {!collapsed && (
          <button
            type="button"
            onClick={toggle}
            className="ml-auto inline-flex items-center justify-center h-6 w-6 rounded text-fg-subtle hover:text-fg-default hover:bg-surface-2 transition-colors"
            title="Collapse sidebar"
            aria-label="Collapse sidebar"
            aria-expanded={true}
          >
            <DoubleArrowLeftIcon className="w-3.5 h-3.5" />
          </button>
        )}
      </div>

      {/* Context block: project + search/command palette */}
      <div className={`shrink-0 ${collapsed ? "px-1.5 py-2" : "px-2 py-2"}`}>
        <SidebarContext collapsed={collapsed} />
      </div>

      {/* Primary nav */}
      <div className={`flex-1 min-h-0 overflow-y-auto ${collapsed ? "px-1.5" : "px-2"}`}>
        <NavLinks collapsed={collapsed} />
      </div>

      {/* Footer: single Settings entry + user/team chip. Backend
          credentials and theme moved into the Settings dialog so the
          two sit together in a coherent panel instead of competing as
          mismatched chrome at the bottom of the sidebar. */}
      <div
        className={`shrink-0 border-t border-border-default ${collapsed ? "px-1.5 py-2 flex flex-col items-center gap-1.5" : "px-2 py-2 flex flex-col gap-1.5"}`}
      >
        {collapsed ? (
          <button
            type="button"
            onClick={openSettings}
            className="inline-flex items-center justify-center h-7 w-7 rounded text-fg-subtle hover:text-fg-default hover:bg-surface-2 transition-colors"
            title="Settings"
            aria-label="Open settings"
          >
            <GearIcon className="w-4 h-4" />
          </button>
        ) : (
          <button
            type="button"
            onClick={openSettings}
            className="flex w-full items-center gap-2 px-2 py-1 text-xs rounded text-fg-default hover:bg-surface-2 transition-colors"
            aria-label="Open settings"
          >
            <GearIcon className="w-4 h-4 shrink-0" />
            <span>Settings</span>
          </button>
        )}
        <UserTeamChip collapsed={collapsed} />
      </div>

      {/* Collapse toggle when collapsed — positioned at the very bottom
          so it doesn't compete visually with the logo (the chevron
          square was reading as a second logo when stacked under it). */}
      {collapsed && (
        <button
          type="button"
          onClick={toggle}
          className="shrink-0 border-t border-border-default h-7 w-full flex items-center justify-center text-fg-subtle hover:text-fg-default hover:bg-surface-2 transition-colors"
          title="Expand sidebar"
          aria-label="Expand sidebar"
          aria-expanded={false}
        >
          <DoubleArrowRightIcon className="w-3.5 h-3.5" />
        </button>
      )}
    </aside>
  );
}
