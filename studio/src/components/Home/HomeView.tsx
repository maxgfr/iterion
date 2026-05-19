import { useRuns } from "@/hooks/useRuns";
import WhatsNextCard from "./WhatsNextCard";
import RecentFilesPanel from "./RecentFilesPanel";
import RunsPanel from "./RunsPanel";

export default function HomeView() {
  const { runs, loading, error } = useRuns({ limit: 50 });

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="max-w-6xl mx-auto space-y-4">
        {/* WhatsNextCard is the curated entry point — full-width above
            the grid so it reads as "start here" rather than as one
            option among many. */}
        <WhatsNextCard />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Both panels span one column each. Cross-store running
              runs are folded into RunsPanel's "In other locations"
              section so the operator sees everything in-flight in a
              single box. */}
          <RecentFilesPanel />
          <RunsPanel runs={runs} loading={loading} error={error} />
        </div>
      </div>
    </div>
  );
}
