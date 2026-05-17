import AppHeader from "@/components/shared/AppHeader";
import { useRuns } from "@/hooks/useRuns";
import PiloteCard from "./PiloteCard";
import RecentFilesPanel from "./RecentFilesPanel";
import RunsPanel from "./RunsPanel";

export default function HomeView() {
  const { runs, loading, error } = useRuns({ limit: 50 });

  return (
    <div className="h-full flex flex-col overflow-hidden bg-surface-0 text-fg-default">
      <AppHeader active="home" />

      <main className="flex-1 overflow-auto p-4 sm:p-6">
        <div className="max-w-6xl mx-auto space-y-4">
          {/* PiloteCard is the curated entry point — full-width above
              the grid so it reads as "start here" rather than as one
              option among many. */}
          <PiloteCard />
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Both panels span one column each. Cross-store running
                runs are folded into RunsPanel's "In other locations"
                section so the operator sees everything in-flight in a
                single box. */}
            <RecentFilesPanel />
            <RunsPanel runs={runs} loading={loading} error={error} />
          </div>
        </div>
      </main>
    </div>
  );
}
