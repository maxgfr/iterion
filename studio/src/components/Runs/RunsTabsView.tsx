import { ListBulletIcon, PlayIcon } from "@radix-ui/react-icons";
import { Suspense, lazy, useCallback, useEffect } from "react";
import { useLocation, useParams } from "wouter";
import { useShallow } from "zustand/react/shallow";

import MainSpinner from "@/components/shared/MainSpinner";
import RunTabHost from "@/components/shared/RunTabHost";
import InnerTabBar from "@/components/shared/InnerTabBar";
import {
  selectRunTabs,
  useTabsStore,
} from "@/store/tabs";

const RunListView = lazy(() => import("@/components/Runs/RunListView"));

// RunsTabsView is mounted on BOTH `/runs` (list mode) and `/runs/:id`
// (single-run mode). The pinned "All runs" tab in the inner strip is
// the list view; clicking it navigates to `/runs` while keeping every
// open run tab visible in the strip — switching back to a run is one
// click. Each open run tab keeps its per-runId store + WS mounted in
// parallel (display:none for the inactive ones) so events accumulate
// in the background.
//
// URL → tab on deep-link via effect; tab → URL on user click in the
// callbacks (see EditorTabsView for the no-bidirectional-effect rule).
export default function RunsTabsView() {
  const params = useParams<{ id: string }>();
  const [, setLocation] = useLocation();
  // See EditorTabsView for the rationale on useShallow.
  const tabs = useTabsStore(useShallow(selectRunTabs));
  const activeTabId = useTabsStore((s) => s.activeRunTabId);
  // pinnedActive is true when the user is on /runs (list view) rather
  // than a specific run; the pinned "All runs" tab highlights and the
  // RunListView fills the content area.
  const pinnedActive = !params.id;

  // URL → tab: only when we have a specific runId in the URL.
  useEffect(() => {
    if (!params.id) return;
    useTabsStore.getState().openTab("run", { runId: params.id });
  }, [params.id]);

  const handleSelect = useCallback(
    (id: string) => {
      useTabsStore.getState().setActive(id);
      const tab = useTabsStore.getState().tabs.find((t) => t.id === id);
      const runId = tab?.params.runId;
      if (!runId) return;
      setLocation(`/runs/${encodeURIComponent(runId)}`, { replace: true });
    },
    [setLocation],
  );

  const handleClose = useCallback(
    (id: string) => {
      useTabsStore.getState().closeTab(id);
      const next = useTabsStore.getState();
      const newActive = next.tabs.find((t) => t.id === next.activeRunTabId);
      const newRunId = newActive?.params.runId;
      setLocation(newRunId ? `/runs/${encodeURIComponent(newRunId)}` : "/runs", {
        replace: true,
      });
    },
    [setLocation],
  );

  const pinnedRunsList = {
    icon: <ListBulletIcon className="w-3.5 h-3.5 shrink-0" />,
    label: "All runs",
    onClick: () => setLocation("/runs"),
    active: pinnedActive,
  };

  return (
    <div className="h-full flex flex-col">
      <InnerTabBar
        tabs={tabs}
        activeTabId={pinnedActive ? null : activeTabId}
        onSelect={handleSelect}
        onClose={handleClose}
        pinnedLead={pinnedRunsList}
        icon={() => <PlayIcon className="w-3.5 h-3.5 shrink-0" />}
      />
      <div className="flex-1 min-h-0 relative">
        <div
          className={`absolute inset-0 ${pinnedActive ? "block" : "hidden"}`}
          aria-hidden={pinnedActive ? undefined : true}
        >
          <Suspense fallback={<MainSpinner />}>
            <RunListView />
          </Suspense>
        </div>
        {tabs.filter((t) => t.hydrated).map((tab) => {
          const runId = tab.params.runId;
          if (!runId) return null;
          const visible = !pinnedActive && tab.id === activeTabId;
          return (
            <div
              key={tab.id}
              className={`absolute inset-0 ${visible ? "block" : "hidden"}`}
              aria-hidden={visible ? undefined : true}
            >
              <RunTabHost runId={runId} tabId={tab.id} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
