// useWatchList tracks dispatcher-bound issues an operator dispatched
// from the current whats-next session. Each entry shows the live state
// of the issue on the native board so the operator can tell at a
// glance whether their dispatch landed, started, or finished — without
// flipping to /board or /runs.
//
// MVP2 layer (held-update semantics): we also keep a per-run buffer
// of state transitions observed while polling. The operator clears
// the buffer via `acknowledgeUpdates()`, which is the seam the
// WatchPanel "Tell Nexie" button uses to forward a batched summary
// into the run inbox. We hold rather than auto-inject so the operator
// stays in control over what Nexie sees on her next turn.
//
// Source of truth (MVP1):
//   * `human_answers_recorded` events on `ask_which_to_process` and
//     `ask_which_to_dispatch_more` — both forms emit
//     `selected_issue_ids: string[]` (the operator's checkbox picks).
//
// MVP3 will move the registry into the runtime; for now we mine it
// from the run event stream the studio already subscribes to.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { getIssue, type NativeIssue } from "@/api/native";
import type { RunEvent } from "@/api/runs";
import { useRunStore } from "@/store/run";

// Node ids whose `selected_issue_ids` answer feeds the watch list.
const DISPATCH_NODE_IDS = new Set<string>([
  "ask_which_to_process",
  "ask_which_to_dispatch_more",
]);

export interface WatchEntry {
  issueId: string;
  // When null the row renders in a "loading" state until the first
  // /issues/<id> fetch resolves.
  issue: NativeIssue | null;
  // Filled when the lookup fails — typically a 404 (issue deleted) or
  // a transient network blip. Surface it muted, not as a red error
  // chip; an out-of-band board edit is benign for the operator.
  lastFetchError?: string;
}

// One observed state transition. The buffer carries only transitions
// the operator hasn't acknowledged yet — once forwarded to Nexie, the
// buffer empties so we don't double-notify on her next turn.
export interface WatchUpdate {
  issueId: string;
  title: string;
  prevState: string;
  newState: string;
  // ISO timestamp at which the studio observed the change. Sourced
  // from the issue's updated_at when available so two studio tabs
  // observing the same change agree on the timeline.
  at: string;
}

export interface UseWatchListResult {
  entries: WatchEntry[];
  pendingUpdates: WatchUpdate[];
  acknowledgeUpdates: () => void;
}

const POLL_INTERVAL_MS = 15_000;

// extractDispatchedIds walks the event stream once and returns a
// deduped ordered list of every issue ID the operator picked through
// a watched human turn during this run.
function extractDispatchedIds(events: ReadonlyArray<RunEvent>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const evt of events) {
    if (evt.type !== "human_answers_recorded") continue;
    if (!evt.node_id || !DISPATCH_NODE_IDS.has(evt.node_id)) continue;
    const answers = evt.data?.answers as Record<string, unknown> | undefined;
    const raw = answers?.["selected_issue_ids"];
    if (!Array.isArray(raw)) continue;
    for (const v of raw) {
      if (typeof v !== "string" || v.length === 0) continue;
      if (seen.has(v)) continue;
      seen.add(v);
      out.push(v);
    }
  }
  return out;
}

export function useWatchList(runId: string | null): UseWatchListResult {
  const events = useRunStore((s) => s.events);

  const watchedIds = useMemo(() => extractDispatchedIds(events), [events]);

  const [byId, setById] = useState<Record<string, WatchEntry>>({});
  const [pendingUpdates, setPendingUpdates] = useState<WatchUpdate[]>([]);

  // Tracks the last state we surfaced to the operator per issue. We
  // use this both for transition detection (poll-N+1 state differs
  // from poll-N) and to seed the operator-visible "starting" state
  // (the first fetch's state is the baseline, not a transition).
  //
  // Persisted to localStorage keyed by the run id so a studio reload
  // mid-session keeps the baseline — without persistence, the post-
  // reload first fetch becomes the new baseline and a transition that
  // happened across the reload boundary is silently lost.
  const lastObservedRef = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    if (!runId) return;
    lastObservedRef.current = loadLastObserved(runId);
  }, [runId]);

  // Reset on run change so a stale Nexie session's watch list doesn't
  // bleed into the next one.
  useEffect(() => {
    setById({});
    setPendingUpdates([]);
  }, [runId]);

  // One interval per watched issue. The pollers live in a ref so
  // React's strict-mode double-mount doesn't double-arm them.
  const pollersRef = useRef<Map<string, ReturnType<typeof setInterval>>>(
    new Map(),
  );

  useEffect(() => {
    if (!runId) return;
    const pollers = pollersRef.current;

    for (const id of watchedIds) {
      if (pollers.has(id)) continue;
      setById((prev) =>
        prev[id] ? prev : { ...prev, [id]: { issueId: id, issue: null } },
      );

      const fetchOnce = async () => {
        try {
          const iss = await getIssue(id);
          // Transition detection: compare to the last observed state.
          // The first fetch seeds the baseline silently; subsequent
          // changes push into the pending-updates buffer.
          const last = lastObservedRef.current.get(id);
          if (last !== undefined && last !== iss.state) {
            const upd: WatchUpdate = {
              issueId: id,
              title: iss.title,
              prevState: last,
              newState: iss.state,
              at: iss.updated_at,
            };
            setPendingUpdates((prev) => [...prev, upd]);
          }
          lastObservedRef.current.set(id, iss.state);
          if (runId) persistLastObserved(runId, lastObservedRef.current);
          setById((prev) => ({
            ...prev,
            [id]: { issueId: id, issue: iss, lastFetchError: undefined },
          }));
        } catch (e) {
          const msg = (e as Error).message ?? String(e);
          setById((prev) => ({
            ...prev,
            [id]: {
              issueId: id,
              issue: prev[id]?.issue ?? null,
              lastFetchError: msg,
            },
          }));
        }
      };
      void fetchOnce();
      pollers.set(id, setInterval(fetchOnce, POLL_INTERVAL_MS));
    }

    for (const [id, timer] of pollers) {
      if (watchedIds.includes(id)) continue;
      clearInterval(timer);
      pollers.delete(id);
      lastObservedRef.current.delete(id);
      if (runId) persistLastObserved(runId, lastObservedRef.current);
    }
  }, [runId, watchedIds]);

  useEffect(() => {
    const pollers = pollersRef.current;
    return () => {
      for (const timer of pollers.values()) clearInterval(timer);
      pollers.clear();
    };
  }, []);

  const acknowledgeUpdates = useCallback(() => {
    setPendingUpdates([]);
    if (runId) persistPendingUpdates(runId, []);
  }, [runId]);

  // Hydrate / persist the pending-updates buffer across reloads so an
  // operator who closes the tab while updates are queued can still
  // forward them on next mount.
  useEffect(() => {
    if (!runId) return;
    setPendingUpdates(loadPendingUpdates(runId));
  }, [runId]);
  useEffect(() => {
    if (!runId) return;
    persistPendingUpdates(runId, pendingUpdates);
  }, [runId, pendingUpdates]);

  const entries = useMemo(
    () =>
      watchedIds
        .map((id) => byId[id])
        .filter((row): row is WatchEntry => row !== undefined),
    [watchedIds, byId],
  );

  return { entries, pendingUpdates, acknowledgeUpdates };
}

// localStorage keys. Keyed by runId so two parallel Nexie sessions
// (different runs) don't share state. Plays nicely with sessionStorage.ts
// which uses bot-id + project-id keys for the run-id mapping itself.
const STORAGE_PREFIX_OBSERVED = "iterion.watchlist.observed:";
const STORAGE_PREFIX_PENDING = "iterion.watchlist.pending:";

function loadLastObserved(runId: string): Map<string, string> {
  try {
    const raw = window.localStorage.getItem(STORAGE_PREFIX_OBSERVED + runId);
    if (!raw) return new Map();
    const parsed = JSON.parse(raw) as Record<string, string>;
    return new Map(Object.entries(parsed));
  } catch {
    return new Map();
  }
}

function persistLastObserved(runId: string, m: Map<string, string>): void {
  try {
    const obj: Record<string, string> = {};
    for (const [k, v] of m) obj[k] = v;
    window.localStorage.setItem(
      STORAGE_PREFIX_OBSERVED + runId,
      JSON.stringify(obj),
    );
  } catch {
    // Quota or denied storage — drop silently; the in-memory ref keeps
    // working for the rest of this session.
  }
}

function loadPendingUpdates(runId: string): WatchUpdate[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_PREFIX_PENDING + runId);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (u): u is WatchUpdate =>
        !!u &&
        typeof u === "object" &&
        typeof u.issueId === "string" &&
        typeof u.title === "string" &&
        typeof u.prevState === "string" &&
        typeof u.newState === "string" &&
        typeof u.at === "string",
    );
  } catch {
    return [];
  }
}

function persistPendingUpdates(runId: string, updates: WatchUpdate[]): void {
  try {
    window.localStorage.setItem(
      STORAGE_PREFIX_PENDING + runId,
      JSON.stringify(updates),
    );
  } catch {
    // ignore quota / denied storage
  }
}

// formatUpdatesAsChatMessage turns a buffered batch of transitions
// into a single operator-flavoured chat line Nexie will see on her
// next turn. Collapses multiple transitions for the same issue to
// the latest one so a backlog→ready→in_progress chain reads as one
// "backlog → in_progress" entry instead of three.
//
// Exported so the panel can preview the text before the operator
// commits, and so tests can pin the wire format.
export function formatUpdatesAsChatMessage(
  updates: ReadonlyArray<WatchUpdate>,
): string {
  if (updates.length === 0) return "";
  // Collapse to one entry per issue, keeping the FIRST seen prevState
  // and the LATEST newState — the operator wants the net delta, not
  // the intermediate hops.
  const collapsed = new Map<
    string,
    { title: string; prevState: string; newState: string }
  >();
  for (const u of updates) {
    const existing = collapsed.get(u.issueId);
    if (existing) {
      existing.newState = u.newState;
      existing.title = u.title;
    } else {
      collapsed.set(u.issueId, {
        title: u.title,
        prevState: u.prevState,
        newState: u.newState,
      });
    }
  }
  const lines: string[] = [];
  lines.push("Board updates since last check:");
  for (const { title, prevState, newState } of collapsed.values()) {
    lines.push(`- ${title}: ${prevState} → ${newState}`);
  }
  return lines.join("\n");
}
