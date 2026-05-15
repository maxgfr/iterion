import { Link, useLocation } from "wouter";

import { Badge } from "@/components/ui/Badge";
import type { RunSummary } from "@/api/runs";
import { formatRelative } from "@/lib/format";
import {
  STATUS_VARIANT,
  labelForStatus,
  isActiveStatus,
} from "@/components/Runs/runStatusMeta";

interface Props {
  runs: RunSummary[];
  loading: boolean;
  error: string | null;
}

const MAX_RECENT_RUNS = 10;

export default function RunsPanel({ runs, loading, error }: Props) {
  const [, setLocation] = useLocation();
  const active = runs.filter((r) => isActiveStatus(r.status));
  const recent = runs
    .filter((r) => !isActiveStatus(r.status))
    .slice(0, MAX_RECENT_RUNS);

  const goToRun = (id: string) =>
    setLocation(`/runs/${encodeURIComponent(id)}`);

  const hasAnything = active.length > 0 || recent.length > 0;

  return (
    <section className="flex flex-col bg-surface-1 border border-border-default rounded-lg overflow-hidden">
      <header className="px-4 py-2.5 border-b border-border-default flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">
          Runs
          {active.length > 0 && (
            <span className="ml-2 inline-flex items-center gap-1 normal-case tracking-normal text-info-fg">
              <span className="inline-block w-1.5 h-1.5 rounded-full bg-info animate-pulse" />
              {active.length} active
            </span>
          )}
        </h2>
        <Link
          href="/runs"
          className="text-xs text-fg-muted hover:text-fg-default"
        >
          View all →
        </Link>
      </header>

      <div className="flex-1 overflow-auto">
        {loading && runs.length === 0 ? (
          <div className="p-4 text-xs text-fg-subtle">Loading…</div>
        ) : error ? (
          <div className="p-4 text-xs text-danger">{error}</div>
        ) : !hasAnything ? (
          <div className="p-4 text-xs text-fg-subtle">
            No runs yet — launch one from the editor.
          </div>
        ) : (
          <ul className="divide-y divide-border-default">
            {active.map((r) => (
              <li key={r.id}>
                <button
                  onClick={() => goToRun(r.id)}
                  className="w-full px-4 py-2.5 flex items-center gap-3 text-left bg-info-soft/30 hover:bg-info-soft/50 border-l-2 border-info"
                >
                  <span
                    className="inline-block w-2 h-2 rounded-full bg-info animate-pulse shrink-0"
                    aria-hidden="true"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-semibold truncate">
                      {r.name || r.workflow_name}
                    </div>
                    {r.name && r.workflow_name !== r.name && (
                      <div className="text-[10px] text-fg-subtle truncate">
                        {r.workflow_name}
                      </div>
                    )}
                  </div>
                  <Badge variant={STATUS_VARIANT[r.status]}>
                    {labelForStatus(r.status)}
                    {r.status === "queued" && r.queue_position
                      ? ` · #${r.queue_position}`
                      : ""}
                  </Badge>
                  <span className="text-[10px] text-fg-subtle shrink-0 w-16 text-right">
                    {formatRelative(r.updated_at)}
                  </span>
                </button>
              </li>
            ))}
            {recent.map((r) => (
              <li key={r.id}>
                <button
                  onClick={() => goToRun(r.id)}
                  className="w-full px-4 py-2 flex items-center gap-3 hover:bg-surface-2 text-left"
                >
                  <div className="min-w-0 flex-1 pl-4">
                    <div className="text-xs font-medium truncate">
                      {r.name || r.workflow_name}
                    </div>
                    {r.name && r.workflow_name !== r.name && (
                      <div className="text-[10px] text-fg-subtle truncate">
                        {r.workflow_name}
                      </div>
                    )}
                  </div>
                  <Badge variant={STATUS_VARIANT[r.status]}>
                    {labelForStatus(r.status)}
                  </Badge>
                  <span className="text-[10px] text-fg-subtle shrink-0 w-16 text-right">
                    {formatRelative(r.updated_at)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
