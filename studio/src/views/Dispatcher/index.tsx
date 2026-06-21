import { errorMessage } from "@/lib/errorHints";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation } from "wouter";

import { useHeaderSlot } from "@/components/shared/useHeaderSlot";
import DispatcherControlBar from "@/components/shared/DispatcherControlBar";
import { Button } from "@/components/ui/Button";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { EmptyState } from "@/components/ui/EmptyState";
import { useConfirm } from "@/hooks/useConfirm";
import {
  cancelIssue,
  getState,
  getStatus,
  openWS,
  refresh as refreshTick,
  reload as reloadConfig,
  type DispatchSkipView,
  type DispatcherSnapshot,
  type ManagerStatus,
} from "@/api/dispatcher";
import SettingsDrawer from "@/components/Dispatcher/SettingsDrawer";
import TrackerErrorBanner from "@/components/shared/TrackerErrorBanner";
import CostCapBanner from "@/components/shared/CostCapBanner";
import { dispatcherActionState } from "./dispatcherActionState";
import { dispatcherPillMeta } from "@/components/shared/dispatcherPillMeta";

export default function DispatcherView() {
  const [, setLocation] = useLocation();
  const [snap, setSnap] = useState<DispatcherSnapshot | null>(null);
  const [status, setStatus] = useState<ManagerStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const { confirm, dialog } = useConfirm();
  const wsRef = useRef<WebSocket | null>(null);
  // The dispatcher manager's WS only exists while it is running or paused;
  // idle / error / absent all 503. Gate the live socket on that so an
  // unconfigured dashboard doesn't spin an endless failed-handshake loop
  // that spams the console (~1 error every 1.5s).
  const dispatcherAttached =
    status?.state === "running" || status?.state === "paused";
  const openRun = useCallback(
    (runID: string) => setLocation(`/runs/${encodeURIComponent(runID)}`),
    [setLocation],
  );

  const reload = useCallback(async () => {
    setError(null);
    try {
      const s = await getState();
      setSnap(s);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    // Only the running/paused manager serves the WS; skip it while idle so
    // we don't loop on 503s. The 2s status poll re-runs this effect once
    // the dispatcher attaches.
    if (!dispatcherAttached) return;
    let cancelled = false;
    let retryTimer: number | null = null;
    let attempt = 0;

    const scheduleRetry = () => {
      if (cancelled) return;
      // Exponential backoff capped at 30s. The prior implementation
      // hammered the server at 1.5s intervals indefinitely whenever
      // the dispatcher was offline.
      const delay = Math.min(1500 * 2 ** attempt, 30_000);
      attempt += 1;
      retryTimer = window.setTimeout(() => void connect(), delay);
    };

    const connect = async () => {
      if (cancelled) return;
      try {
        const ws = await openWS();
        if (cancelled) {
          ws.close();
          return;
        }
        wsRef.current = ws;
        // Reset the backoff only on a real connection: openWS resolves a
        // still-connecting socket, so a 503 handshake failure surfaces
        // later via onerror/onclose. Resetting here would defeat the
        // backoff and retry every 1.5s.
        ws.onopen = () => {
          attempt = 0;
        };
        ws.onmessage = (e) => {
          try {
            setSnap(JSON.parse(e.data) as DispatcherSnapshot);
          } catch {
            // ignore malformed frames
          }
        };
        ws.onclose = () => {
          if (cancelled) return;
          // Drop our ref so a stale handler can't observe the closed
          // socket as "live" on the next render.
          if (wsRef.current === ws) wsRef.current = null;
          scheduleRetry();
        };
        ws.onerror = () => ws.close();
      } catch (e) {
        setError(errorMessage(e));
        scheduleRetry();
      }
    };
    void connect();
    return () => {
      cancelled = true;
      if (retryTimer != null) {
        window.clearTimeout(retryTimer);
        retryTimer = null;
      }
      const ws = wsRef.current;
      wsRef.current = null;
      if (ws) {
        // Detach close handler so the in-flight FIN doesn't kick off
        // a new reconnect cycle after the effect tore down.
        ws.onclose = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.close();
      }
    };
  }, [reload, dispatcherAttached]);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await getStatus();
        if (!cancelled) setStatus(s);
      } catch {
        if (!cancelled) setStatus(null);
      }
    };
    void tick();
    const t = window.setInterval(() => void tick(), 2000);
    return () => {
      cancelled = true;
      window.clearInterval(t);
    };
  }, []);

  const actions = dispatcherActionState(status?.state, snap?.paused ?? false);

  const doRefresh = useCallback(async () => {
    try {
      await refreshTick();
    } catch (e) {
      setError(errorMessage(e));
    }
  }, []);
  const doReload = useCallback(async () => {
    try {
      await reloadConfig();
    } catch (e) {
      setError(errorMessage(e));
    }
  }, []);
  const doCancel = useCallback(
    async (issueID: string) => {
      const ok = await confirm({
        title: "Cancel in-flight run?",
        message: `Cancel the in-flight run for ${issueID}? The issue stays on the board and can be re-dispatched.`,
        confirmLabel: "Cancel run",
        confirmVariant: "danger",
      });
      if (!ok) return;
      try {
        await cancelIssue(issueID);
      } catch (e) {
        setError(errorMessage(e));
      }
    },
    [confirm],
  );

  useHeaderSlot({
    left: <span className="text-xs font-medium text-fg-default">Dispatcher</span>,
    right: (
      <>
        <Button
          variant="secondary"
          size="sm"
          disabled={!actions.canPollDispatches}
          title={actions.pollTitle}
          onClick={() => void doRefresh()}
        >
          Poll now
        </Button>
        <Button
          variant="secondary"
          size="sm"
          disabled={!actions.canReloadConfig}
          title={actions.reloadTitle}
          onClick={() => void doReload()}
        >
          Reload config
        </Button>
      </>
    ),
  });

  if (loading) {
    return <EmptyState message="Loading dispatcher state…" />;
  }
  if (!snap) {
    return (
      <div className="p-8 text-fg-muted">
        Dispatcher not available.{" "}
        <code className="text-xs">iterion dispatch &lt;config.yaml&gt;</code> exposes this view.
      </div>
    );
  }

  const running = snap.running ?? [];
  const retries = snap.retries ?? [];
  // Eligible issues the dispatcher refused to claim this scan because
  // their explicit `bot` is unresolvable / unrouteable. Surfaced here so
  // a misconfigured `bot:` is visible on the dashboard too (the board
  // already badges the affected cards) — otherwise an operator watching
  // this view sees an eligible issue that simply never runs, with no
  // reason given.
  const skips = snap.dispatch_skips ?? [];

  return (
    <div className="h-full flex flex-col overflow-hidden">
      {dialog}
      <DispatcherControlBar onOpenSettings={() => setSettingsOpen(true)} />
      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={() => void reload()}
      />

      {error && <InlineBanner tone="danger">{error}</InlineBanner>}

      {snap.last_tracker_error && (
        <TrackerErrorBanner
          tracker={snap.tracker}
          message={snap.last_tracker_error}
        />
      )}

      {/* Daily spend-cap banner — self-polls /api/v1/limits/cost and
          renders only when the cap is reached. The dispatcher gate has
          already stopped new dispatches; this surfaces it + the
          override action. */}
      <CostCapBanner />

      {snap.paused && (
        <InlineBanner tone="warning" title="Dispatcher paused">
          New dispatches are suspended. The retry queue won't fire, and new ready
          issues won't be picked up. In-flight runs continue. Resume from the toolbar above.
        </InlineBanner>
      )}

      <div className="flex-1 overflow-auto p-4 space-y-4 max-w-4xl">
        <SummaryCard snap={snap} status={status} />
        <DispatchSkipsTable
          rows={skips}
          onFocusIssue={(id) =>
            setLocation(`/board?focus=${encodeURIComponent(id)}`)
          }
        />
        <RunningTable
          rows={running}
          stallTimeoutS={snap.stall_timeout_seconds}
          onCancel={doCancel}
          onOpenRun={openRun}
        />
        <RetriesTable
          rows={retries}
          canPollDispatches={actions.canPollDispatches}
          pollTitle={actions.pollTitle}
          onFocusIssue={(id) =>
            setLocation(`/board?focus=${encodeURIComponent(id)}`)
          }
          onRefreshNow={() => void doRefresh()}
        />
      </div>
    </div>
  );
}

function SummaryCard({
  snap,
  status,
}: {
  snap: DispatcherSnapshot;
  status: ManagerStatus | null;
}) {
  // Chip reflects manager LIFECYCLE state (idle/running/paused/error),
  // not just the actor's runtime `paused` flag. Previously the chip
  // read `snap.paused` only, so it showed "active" whenever the actor
  // existed but wasn't operator-paused — including transient windows
  // where the manager was idle/error/initialising. Reuse the same pill
  // metadata as the DispatcherControlBar so both chips stay in sync.
  const pillState = status ? status.state : "unreachable";
  const meta = dispatcherPillMeta(pillState);
  return (
    <section className="rounded border border-border-default bg-surface-1 p-4">
      <div className="flex items-center justify-between mb-2 gap-3">
        <h2 className="text-sm font-semibold">{snap.name || "Dispatcher"}</h2>
        <span
          className={`text-caption font-mono rounded px-1.5 py-0.5 ${meta.className}`}
          title={meta.title}
        >
          {pillState}
        </span>
      </div>
      <dl className="grid grid-cols-2 sm:grid-cols-4 gap-x-4 gap-y-1 text-xs">
        <KV k="Tracker" v={snap.tracker} />
        <KV k="Polling" v={`${snap.polling_interval_seconds}s`} />
        <KV k="Stall timeout" v={`${snap.stall_timeout_seconds}s`} />
        <KV k="Slots" v={`${snap.slots.global_used} / ${snap.slots.global_max}`} />
      </dl>
      {snap.slots.per_state_max && Object.keys(snap.slots.per_state_max).length > 0 && (
        <div className="mt-2 text-xs text-fg-muted">
          per-state caps:{" "}
          {Object.entries(snap.slots.per_state_max).map(([s, max]) => (
            <span key={s} className="ml-2">
              {s}: {snap.slots.per_state_used?.[s] ?? 0}/{max}
            </span>
          ))}
        </div>
      )}
    </section>
  );
}

// stallStyle inspects how long a running entry has been silent relative
// to the dispatcher's stallTimeout and returns a (className, hint) pair
// for the row. The thresholds match what the operator can act on:
//   ≥ 50% of the budget elapsed → amber  ("slow — keep an eye")
//   ≥ 100% of the budget        → red    ("about to be cancelled")
// Below 50%: no decoration, the row reads as normal.
function stallStyle(
  lastEventAt: string,
  stallTimeoutS: number,
): { rowClass: string; hint: string | null } {
  if (!stallTimeoutS || stallTimeoutS <= 0) {
    return { rowClass: "", hint: null };
  }
  const last = Date.parse(lastEventAt);
  if (!Number.isFinite(last)) return { rowClass: "", hint: null };
  const elapsedS = (Date.now() - last) / 1000;
  if (elapsedS >= stallTimeoutS) {
    return {
      rowClass: "bg-danger-soft",
      hint: `Silent for ${Math.round(elapsedS)}s ≥ stall timeout (${Math.round(stallTimeoutS)}s) — will be cancelled on the next reconciliation tick.`,
    };
  }
  if (elapsedS >= stallTimeoutS / 2) {
    return {
      rowClass: "bg-warning-soft",
      hint: `Silent for ${Math.round(elapsedS)}s — half the stall budget (${Math.round(stallTimeoutS)}s) consumed.`,
    };
  }
  return { rowClass: "", hint: null };
}

function RunningTable({
  rows,
  stallTimeoutS,
  onCancel,
  onOpenRun,
}: {
  rows: DispatcherSnapshot["running"];
  stallTimeoutS: number;
  onCancel: (id: string) => void;
  onOpenRun: (runID: string) => void;
}) {
  return (
    <section className="rounded border border-border-default bg-surface-1">
      <header className="px-4 py-2 border-b border-border-default text-sm font-semibold">
        Running ({rows?.length ?? 0})
      </header>
      {!rows || rows.length === 0 ? (
        <div className="p-4 text-xs text-fg-muted">No runs in flight.</div>
      ) : (
        // overflow-x-auto so the 7-column row stays fully reachable
        // when the page is capped by max-w-4xl on small viewports.
        <div className="overflow-x-auto">
        <table className="min-w-full text-xs">
          <thead className="text-fg-muted border-b border-border-default">
            <tr>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Identifier</th>
              <th className="text-left py-1.5 px-3 font-normal">Run</th>
              <th className="text-left py-1.5 px-3 font-normal">State</th>
              <th className="text-left py-1.5 px-3 font-normal">Workspace</th>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Started</th>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Last event</th>
              <th className="text-right py-1.5 px-3 font-normal">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows!.map((r) => {
              const stall = stallStyle(r.last_event_at, stallTimeoutS);
              return (
              <tr
                key={r.issue_id}
                className={`border-b border-border-default/60 ${stall.rowClass}`}
                title={stall.hint ?? undefined}
              >
                <td className="py-1.5 px-3 font-mono whitespace-nowrap">{r.identifier}</td>
                <td className="py-1.5 px-3 font-mono truncate max-w-[14rem]">
                  <button
                    type="button"
                    onClick={() => onOpenRun(r.run_id)}
                    className="text-info hover:underline"
                    title={`Open run ${r.run_id}`}
                  >
                    {r.run_id}
                  </button>
                  {r.attempt && r.attempt > 0 ? (
                    <span
                      className="ml-1.5 inline-flex items-center rounded bg-warning-soft text-warning-fg px-1.5 py-0.5 text-caption font-mono align-middle"
                      title={`Resume of a prior failed_resumable run — attempt ${r.attempt + 1}. The dispatcher continues from the failing node's checkpoint instead of starting fresh.`}
                    >
                      resume #{r.attempt + 1}
                    </span>
                  ) : null}
                </td>
                <td className="py-1.5 px-3">{r.workflow_state}</td>
                <td
                  className="py-1.5 px-3 font-mono text-fg-muted truncate max-w-[18rem]"
                  title={r.workspace_path ?? "no workspace path captured (legacy or in-process run)"}
                >
                  {r.workspace_path ? compactWorkspace(r.workspace_path) : <span className="text-fg-subtle">—</span>}
                </td>
                <td className="py-1.5 px-3 text-fg-muted whitespace-nowrap">{relTime(r.started_at)}</td>
                <td className="py-1.5 px-3 text-fg-muted whitespace-nowrap">
                  {r.last_event_name ? r.last_event_name + " · " : ""}
                  {relTime(r.last_event_at)}
                  {stall.hint && (
                    <span className="ml-1 text-warning-fg/90">⏱</span>
                  )}
                </td>
                <td className="py-1.5 px-3 text-right">
                  <button
                    onClick={() => onCancel(r.issue_id)}
                    className="text-micro px-2 py-0.5 rounded border border-border-default hover:bg-surface-2"
                  >
                    Cancel
                  </button>
                </td>
              </tr>
              );
            })}
          </tbody>
        </table>
        </div>
      )}
    </section>
  );
}

// useTick re-renders the caller at intervalMs while `active`. Used by
// RetriesTable to keep countdowns smooth without a full dispatcher poll
// each second — the retry table only needs to recompute due_at minus
// now() on its own clock.
function useTick(intervalMs: number, active: boolean): number {
  const [tick, setTick] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => setTick(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, active]);
  return tick;
}

// formatRetryDue returns a short human label for "in 12s" / "due now"
// derived purely from due_at + now. Lives next to RetriesTable so the
// formatting stays scoped to the retry context (the rest of the page
// uses relTime).
function formatRetryDue(dueIso: string, nowMs: number): string {
  if (!dueIso) return "";
  const due = Date.parse(dueIso);
  if (!Number.isFinite(due)) return "";
  const deltaS = Math.round((due - nowMs) / 1000);
  if (deltaS <= 0) return "due";
  if (deltaS < 60) return `in ${deltaS}s`;
  if (deltaS < 3600) return `in ${Math.round(deltaS / 60)}m`;
  return `in ${Math.round(deltaS / 3600)}h`;
}

function RetriesTable({
  rows,
  canPollDispatches,
  pollTitle,
  onFocusIssue,
  onRefreshNow,
}: {
  rows: DispatcherSnapshot["retries"];
  canPollDispatches: boolean;
  pollTitle: string;
  onFocusIssue: (issueID: string) => void;
  onRefreshNow: () => void;
}) {
  // Tick every 1s when at least one retry is due in under 5 minutes so
  // the countdown is responsive without burning CPU on long-deferred
  // queues.
  const needsTick = (rows ?? []).some((r) => {
    const due = Date.parse(r.due_at);
    return Number.isFinite(due) && due - Date.now() < 5 * 60_000;
  });
  const now = useTick(1000, needsTick);
  return (
    <section className="rounded border border-border-default bg-surface-1">
      <header className="px-4 py-2 border-b border-border-default text-sm font-semibold flex items-center justify-between gap-2">
        <span>Retry queue ({rows?.length ?? 0})</span>
        {rows && rows.length > 0 && (
          <button
            type="button"
            onClick={onRefreshNow}
            disabled={!canPollDispatches}
            className="text-micro px-2 py-0.5 rounded border border-border-default hover:bg-surface-2 text-fg-muted hover:text-fg-default disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-transparent disabled:hover:text-fg-muted"
            title={pollTitle}
          >
            Poll now
          </button>
        )}
      </header>
      {!rows || rows.length === 0 ? (
        <div className="p-4 text-xs text-fg-muted">No retries pending.</div>
      ) : (
        <div className="overflow-x-auto">
        <table className="min-w-full text-xs">
          <thead className="text-fg-muted border-b border-border-default">
            <tr>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Issue</th>
              <th className="text-left py-1.5 px-3 font-normal">Attempt</th>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Due</th>
              <th className="text-left py-1.5 px-3 font-normal">Last error</th>
            </tr>
          </thead>
          <tbody>
            {rows!.map((r) => {
              const dueLabel = formatRetryDue(r.due_at, now);
              const isDue = dueLabel === "due";
              return (
              <tr
                key={r.issue_id}
                className={`border-b border-border-default/60 hover:bg-surface-2/40 cursor-pointer ${
                  isDue ? "bg-warning-soft" : ""
                }`}
                onClick={() => onFocusIssue(r.issue_id)}
                title="Open this issue on the board"
              >
                <td className="py-1.5 px-3 font-mono whitespace-nowrap">{r.identifier || r.issue_id}</td>
                <td className="py-1.5 px-3">{r.attempt}</td>
                <td className="py-1.5 px-3 whitespace-nowrap">
                  <span className={isDue ? "text-warning-fg" : "text-fg-muted"}>
                    {dueLabel || relTime(r.due_at)}
                  </span>
                </td>
                <td className="py-1.5 px-3 text-danger-fg/80 truncate max-w-[24rem]">
                  {r.error}
                </td>
              </tr>
              );
            })}
          </tbody>
        </table>
        </div>
      )}
    </section>
  );
}

// DispatchSkipsTable surfaces eligible issues the dispatcher refused to
// claim because their explicit `bot` can't be resolved to a workflow file
// or has no dispatch route (not in assignee_workflows). Mirrors the board's
// per-card "won't dispatch" badge so the failure is visible on the
// dashboard too — without it, the dispatch_skips snapshot field (already
// wired through state.go + the WS payload) reached only the board, and an
// operator watching this view saw an eligible issue silently never run.
//
// Renders nothing when there are no skips (the common case) — this is an
// exceptional misconfiguration surface, not a steady-state queue like
// running/retries, so it stays out of the way until it has something to
// say. Rows are click-to-focus the offending issue on the board, where the
// editor lets the operator fix the bot.
function DispatchSkipsTable({
  rows,
  onFocusIssue,
}: {
  rows: DispatchSkipView[];
  onFocusIssue: (issueID: string) => void;
}) {
  if (rows.length === 0) return null;
  return (
    <section className="rounded border border-danger/40 bg-danger-soft">
      <header className="px-4 py-2 border-b border-danger/30 text-sm font-semibold text-danger-fg flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span>⚠ Won&apos;t dispatch ({rows.length})</span>
        <span className="text-micro font-normal text-danger-fg/70">
          eligible issues the dispatcher refuses to claim — fix the{" "}
          <code>bot</code> in the issue editor or add it to{" "}
          <code>assignee_workflows</code>
        </span>
      </header>
      <div className="overflow-x-auto">
        <table className="min-w-full text-xs">
          <thead className="text-fg-muted border-b border-border-default">
            <tr>
              <th className="text-left py-1.5 px-3 font-normal whitespace-nowrap">Issue</th>
              <th className="text-left py-1.5 px-3 font-normal">Bot</th>
              <th className="text-left py-1.5 px-3 font-normal">Reason</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr
                key={s.issue_id}
                className="border-b border-border-default/60 hover:bg-surface-2/40 cursor-pointer"
                onClick={() => onFocusIssue(s.issue_id)}
                title="Open this issue on the board to fix its bot"
              >
                <td className="py-1.5 px-3 font-mono whitespace-nowrap">
                  {s.identifier || s.issue_id}
                </td>
                <td className="py-1.5 px-3 font-mono">
                  {s.bot ? s.bot : <span className="text-fg-subtle">—</span>}
                </td>
                <td className="py-1.5 px-3 text-danger-fg/80">{s.reason}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <>
      <dt className="text-fg-muted">{k}</dt>
      <dd>{v}</dd>
    </>
  );
}

// compactWorkspace trims the noisy `<store-root>/dispatcher/workspaces/`
// prefix common to all dispatcher-driven worktrees so the column reads
// as the issue's identifier suffix at a glance. Leaves arbitrary host
// paths intact (cloud runner pods, manually-launched runs) so the
// column never lies about what's on disk.
function compactWorkspace(path: string): string {
  const m = path.match(/dispatcher\/workspaces\/(.+)$/);
  if (m && m[1]) return m[1];
  // Worktrees laid out by runtime.WithWorktree (not dispatcher) live
  // under `<store-root>/worktrees/<run-id>`; show only the run-id.
  const w = path.match(/\/worktrees\/(.+)$/);
  if (w && w[1]) return w[1];
  return path;
}

function relTime(iso: string): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return iso;
  const dt = (Date.now() - t) / 1000;
  if (dt < 0) {
    const abs = Math.abs(dt);
    if (abs < 60) return `in ${Math.round(abs)}s`;
    if (abs < 3600) return `in ${Math.round(abs / 60)}m`;
    return `in ${Math.round(abs / 3600)}h`;
  }
  if (dt < 60) return `${Math.round(dt)}s ago`;
  if (dt < 3600) return `${Math.round(dt / 60)}m ago`;
  return `${Math.round(dt / 3600)}h ago`;
}
