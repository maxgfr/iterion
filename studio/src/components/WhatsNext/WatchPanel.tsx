// WatchPanel surfaces the dispatcher-bound issues the operator picked
// from this whats-next session. Each row shows the live board state
// of the issue plus a console-link to its most recent dispatcher run,
// so the operator can tell what landed / is running / finished
// without flipping to /board.
//
// MVP2: when the poller catches a state transition (in_progress →
// review → done, …) we buffer it in the hook. The panel surfaces an
// updates badge + a "Tell Nexie" button that forwards a collapsed
// summary into the run inbox via queueMessage — so the operator
// stays in control of what crosses into Nexie's next turn.
//
// MVP3 will move the registry into the runtime; the WatchPanel's
// rendering contract stays stable across that revision.

import { useState } from "react";
import { Link } from "wouter";

import { queueMessage } from "@/api/queueMessages";
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
    const text = formatUpdatesAsChatMessage(pendingUpdates);
    try {
      await queueMessage(runId, text);
      acknowledgeUpdates();
    } catch (e) {
      // Keep the buffer so the operator can retry; surface the error
      // muted so it doesn't dominate the panel.
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
          <Link
            href="/board"
            className="text-[10px] text-accent hover:underline"
          >
            board ↗
          </Link>
        </div>
      </div>
      {forwardError && (
        <div
          className="mb-1 text-[10px] text-red-300"
          title={forwardError}
        >
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
  const title = issue?.title ?? truncateId(issueId);
  const state = issue?.state ?? "…";
  const lastRunId = issue?.last_run_id ?? null;

  return (
    <li className="flex items-center gap-2 text-[12px] text-fg-default">
      <StateChip state={state} />
      <span className="flex-1 min-w-0 truncate" title={title}>
        {title}
      </span>
      {lastFetchError && (
        <span
          className="text-[10px] text-fg-subtle"
          title={lastFetchError}
        >
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

// StateChip color-encodes the native board state into a compact badge.
// The palette mirrors the board view's column header tones so an
// operator who's been staring at /board recognises the state at a
// glance. Unknown states get a neutral chip rather than a missing one
// — the chip is the alignment anchor for the row.
function StateChip({ state }: { state: string }) {
  const cls = chipClasses(state);
  return (
    <span
      className={`shrink-0 inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium uppercase tracking-wide ${cls}`}
    >
      {state}
    </span>
  );
}

function chipClasses(state: string): string {
  switch (state) {
    case "backlog":
      return "bg-surface-2 text-fg-muted";
    case "ready":
      return "bg-accent-soft text-accent";
    case "in_progress":
      return "bg-amber-500/15 text-amber-300";
    case "review":
      return "bg-violet-500/15 text-violet-300";
    case "done":
      return "bg-emerald-500/15 text-emerald-300";
    case "failed":
    case "cancelled":
      return "bg-red-500/15 text-red-300";
    default:
      return "bg-surface-2 text-fg-subtle";
  }
}

function truncateId(id: string): string {
  return id.replace(/^native:/, "").slice(0, 8);
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}
