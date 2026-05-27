import { useState } from "react";
import { Link } from "wouter";

import { queueMessage } from "@/api/queueMessages";
import { stateChipStyle } from "@/lib/board/stateTheme";
import { shortIssueId } from "@/lib/whats-next/issueId";
import {
  formatUpdatesAsChatMessage,
  useWatchList,
  type WatchEntry,
} from "@/lib/whats-next/useWatchList";

interface WatchPanelProps {
  runId: string | null;
}

export default function WatchPanel({ runId }: WatchPanelProps) {
  const { entries, pendingUpdates, acknowledgeUpdates } = useWatchList(runId);

  const [forwardError, setForwardError] = useState<string | null>(null);
  const [forwarding, setForwarding] = useState(false);

  if (entries.length === 0) return null;

  const onTellNexie = async () => {
    if (!runId || pendingUpdates.length === 0) return;
    setForwardError(null);
    setForwarding(true);
    try {
      await queueMessage(runId, formatUpdatesAsChatMessage(pendingUpdates));
      acknowledgeUpdates();
    } catch (e) {
      setForwardError((e as Error).message ?? String(e));
    } finally {
      setForwarding(false);
    }
  };

  return (
    <div
      className="border-b border-border-subtle bg-surface-1/60 px-4 py-2"
      role="region"
      aria-label="Dispatched items"
    >
      <div className="flex items-center justify-between gap-2 mb-1">
        <div className="text-[10px] uppercase tracking-wide text-fg-subtle">
          Watching · {entries.length} dispatched
          {pendingUpdates.length > 0 && (
            <span className="ml-2 text-amber-300 normal-case tracking-normal">
              · {pendingUpdates.length} update{pendingUpdates.length === 1 ? "" : "s"}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {pendingUpdates.length > 0 && runId && (
            <button
              type="button"
              onClick={() => void onTellNexie()}
              disabled={forwarding}
              className="text-[11px] text-accent hover:underline cursor-pointer disabled:opacity-50 disabled:cursor-wait"
              title="Forward these state changes into Nexie's chat so she can react on her next turn."
            >
              {forwarding ? "Forwarding…" : "Tell Nexie"}
            </button>
          )}
          <Link href="/board" className="text-[10px] text-accent hover:underline">
            board ↗
          </Link>
        </div>
      </div>
      {forwardError && (
        <div className="mb-1 text-[10px] text-red-300" title={forwardError}>
          Forward failed: {truncate(forwardError, 80)}
        </div>
      )}
      <ul className="flex flex-col gap-1">
        {entries.map((entry) => (
          <WatchRow key={entry.issueId} entry={entry} />
        ))}
      </ul>
    </div>
  );
}

function WatchRow({ entry }: { entry: WatchEntry }) {
  const { issue, issueId, lastFetchError } = entry;
  const title = issue?.title ?? shortIssueId(issueId);
  const state = issue?.state ?? "…";
  const lastRunId = issue?.last_run_id ?? null;

  return (
    <li className="flex items-center gap-2 text-[12px] text-fg-default">
      <span
        className="shrink-0 inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium uppercase tracking-wide"
        style={stateChipStyle(state)}
      >
        {state}
      </span>
      <span className="flex-1 min-w-0 truncate" title={title}>
        {title}
      </span>
      {lastFetchError && (
        <span className="text-[10px] text-fg-subtle" title={lastFetchError}>
          (stale)
        </span>
      )}
      {lastRunId && (
        <Link
          href={`/runs/${encodeURIComponent(lastRunId)}`}
          className="text-[11px] text-accent hover:underline shrink-0"
          title={`Open last run ${lastRunId}`}
        >
          run ↗
        </Link>
      )}
    </li>
  );
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}
