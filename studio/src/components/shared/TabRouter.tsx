import { useMemo, type ReactNode } from "react";

import RunTabHost from "./RunTabHost";
import EditorTabHost from "./EditorTabHost";
import { useTabsStore } from "@/store/tabs";

// TabRouter renders one tab subtree at a time for non-run kinds, and
// keeps every hydrated run tab mounted in parallel (hidden via
// display: none for the non-active ones). The wouter <Switch> upstream
// remains the renderer for non-tab routes (/runs/new, etc.) — see
// AppShell. Background-mounted run subtrees keep their WebSocket
// connections alive so live events accumulate even when the user is
// looking at a different tab.
//
// Non-run kinds (home, editor, runs, whats-next, board, dispatcher,
// settings, team) are still served by wouter's <Switch> in Phase 2 —
// the URL is kept in sync with the active tab, so each route mounts
// the matching view. This will change in Phase 3 when EditorTabHost
// lands and editor tabs gain isolated state too.
export default function TabRouter({ fallback }: { fallback: ReactNode }) {
  const tabs = useTabsStore((s) => s.tabs);
  const activeTabId = useTabsStore((s) => s.activeTabId);
  const activeKind = tabs.find((t) => t.id === activeTabId)?.kind;
  // Memoise the filtered lists by tabs identity so unrelated tab
  // store updates (e.g. label rename) don't force a re-render of
  // every hosted subtree — only structural changes do.
  const runTabs = useMemo(
    () => tabs.filter((t) => t.kind === "run" && t.hydrated && t.params.runId),
    [tabs],
  );
  const editorTabs = useMemo(
    () => tabs.filter((t) => t.kind === "editor" && t.hydrated),
    [tabs],
  );
  const fallbackHidden = activeKind === "run" || activeKind === "editor";

  return (
    <>
      {/* Run tabs: every hydrated one stays mounted; only the active
          one is visible. Hidden tabs keep their WS connection so
          events accumulate while the user is looking elsewhere. */}
      {runTabs.map((tab) => (
        <div
          key={tab.id}
          className={`h-full w-full ${tab.id === activeTabId ? "block" : "hidden"}`}
          aria-hidden={tab.id === activeTabId ? undefined : true}
        >
          <RunTabHost runId={tab.params.runId!} />
        </div>
      ))}
      {/* Editor tabs: every hydrated one stays mounted with its own
          document + selection store. Hidden tabs preserve dirty state,
          undo history, and canvas scroll so switching feels instant. */}
      {editorTabs.map((tab) => (
        <div
          key={tab.id}
          className={`h-full w-full ${tab.id === activeTabId ? "block" : "hidden"}`}
          aria-hidden={tab.id === activeTabId ? undefined : true}
        >
          <EditorTabHost tabId={tab.id} file={tab.params.file} />
        </div>
      ))}
      {/* Non-tab-hosted kinds (Home, Runs list, Board, Dispatcher,
          Settings, Team, LaunchView on /runs/new): served by the
          wouter <Switch> fallback. Hidden whenever a hosted tab kind
          (run or editor) is active so the per-tab view is the only
          thing visible. */}
      <div className={`h-full w-full ${fallbackHidden ? "hidden" : "block"}`}>
        {fallback}
      </div>
    </>
  );
}

