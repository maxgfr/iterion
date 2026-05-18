import { useCallback, useEffect, useRef, useState } from "react";

import PageShell from "@/components/shared/PageShell";
import ConductorControlBar from "@/components/shared/ConductorControlBar";
import { Button } from "@/components/ui/Button";
import {
  cancelIssue,
  getState,
  getStatus,
  openWS,
  refresh as refreshTick,
  reload as reloadConfig,
  type ConductorSnapshot,
  type ManagerStatus,
} from "@/api/conductor";
import SettingsDrawer from "./SettingsDrawer";

export default function ConductorView() {
  const [snap, setSnap] = useState<ConductorSnapshot | null>(null);
  const [status, setStatus] = useState<ManagerStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const s = await getState();
      setSnap(s);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    let cancelled = false;
    let retryTimer: number | null = null;
    let attempt = 0;

    const scheduleRetry = () => {
      if (cancelled) return;
      // Exponential backoff capped at 30s. The prior implementation
      // hammered the server at 1.5s intervals indefinitely whenever
      // the conductor was offline.
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
        attempt = 0;
        wsRef.current = ws;
        ws.onmessage = (e) => {
          try {
            setSnap(JSON.parse(e.data) as ConductorSnapshot);
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
        setError(e instanceof Error ? e.message : String(e));
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
  }, [reload]);

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

  const canOperate = status?.state === "running" || status?.state === "paused";

  const doRefresh = useCallback(async () => {
    try {
      await refreshTick();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);
  const doReload = useCallback(async () => {
    try {
      await reloadConfig();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);
  const doCancel = useCallback(async (issueID: string) => {
    if (!confirm(`Cancel run for ${issueID}?`)) return;
    try {
      await cancelIssue(issueID);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  if (loading) {
    return (
      <PageShell active="conductor">
        <div className="p-8 text-fg-muted">Loading conductor state…</div>
      </PageShell>
    );
  }
  if (!snap) {
    return (
      <PageShell active="conductor">
        <div className="p-8 text-fg-muted">
          Conductor not available.{" "}
          <code className="text-xs">iterion conduct &lt;config.yaml&gt;</code> exposes this view.
        </div>
      </PageShell>
    );
  }

  const running = snap.running ?? [];
  const retries = snap.retries ?? [];

  return (
    <PageShell
      active="conductor"
      rightActions={
        <>
          <Button
            variant="secondary"
            size="sm"
            disabled={!canOperate}
            title={canOperate ? undefined : "Start the conductor first"}
            onClick={() => void doRefresh()}
          >
            Force tick
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={!canOperate}
            title={canOperate ? undefined : "Start the conductor first"}
            onClick={() => void doReload()}
          >
            Reload config
          </Button>
        </>
      }
    >
      <ConductorControlBar onOpenSettings={() => setSettingsOpen(true)} />
      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={() => void reload()}
      />

      {error && (
        <div className="bg-red-500/10 border-b border-red-500/40 px-4 py-2 text-xs text-red-200">
          {error}
        </div>
      )}

      <main className="flex-1 overflow-auto p-4 space-y-4 max-w-4xl">
        <SummaryCard snap={snap} />
        <RunningTable rows={running} onCancel={doCancel} />
        <RetriesTable rows={retries} />
      </main>
    </PageShell>
  );
}

function SummaryCard({ snap }: { snap: ConductorSnapshot }) {
  return (
    <section className="rounded border border-border-default bg-surface-1 p-4">
      <h2 className="text-sm font-semibold mb-2">{snap.name || "Conductor"}</h2>
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

function RunningTable({
  rows,
  onCancel,
}: {
  rows: ConductorSnapshot["running"];
  onCancel: (id: string) => void;
}) {
  return (
    <section className="rounded border border-border-default bg-surface-1">
      <header className="px-4 py-2 border-b border-border-default text-sm font-semibold">
        Running ({rows?.length ?? 0})
      </header>
      {!rows || rows.length === 0 ? (
        <div className="p-4 text-xs text-fg-muted">No runs in flight.</div>
      ) : (
        <table className="w-full text-xs">
          <thead className="text-fg-muted border-b border-border-default">
            <tr>
              <th className="text-left py-1.5 px-3 font-normal">Identifier</th>
              <th className="text-left py-1.5 px-3 font-normal">Run</th>
              <th className="text-left py-1.5 px-3 font-normal">State</th>
              <th className="text-left py-1.5 px-3 font-normal">Started</th>
              <th className="text-left py-1.5 px-3 font-normal">Last event</th>
              <th className="text-right py-1.5 px-3 font-normal">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows!.map((r) => (
              <tr key={r.issue_id} className="border-b border-border-default/60">
                <td className="py-1.5 px-3 font-mono">{r.identifier}</td>
                <td className="py-1.5 px-3 font-mono truncate max-w-[14rem]">{r.run_id}</td>
                <td className="py-1.5 px-3">{r.workflow_state}</td>
                <td className="py-1.5 px-3 text-fg-muted">{relTime(r.started_at)}</td>
                <td className="py-1.5 px-3 text-fg-muted">
                  {r.last_event_name ? r.last_event_name + " · " : ""}
                  {relTime(r.last_event_at)}
                </td>
                <td className="py-1.5 px-3 text-right">
                  <button
                    onClick={() => onCancel(r.issue_id)}
                    className="text-[11px] px-2 py-0.5 rounded border border-border-default hover:bg-surface-2"
                  >
                    Cancel
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function RetriesTable({ rows }: { rows: ConductorSnapshot["retries"] }) {
  return (
    <section className="rounded border border-border-default bg-surface-1">
      <header className="px-4 py-2 border-b border-border-default text-sm font-semibold">
        Retry queue ({rows?.length ?? 0})
      </header>
      {!rows || rows.length === 0 ? (
        <div className="p-4 text-xs text-fg-muted">No retries pending.</div>
      ) : (
        <table className="w-full text-xs">
          <thead className="text-fg-muted border-b border-border-default">
            <tr>
              <th className="text-left py-1.5 px-3 font-normal">Issue</th>
              <th className="text-left py-1.5 px-3 font-normal">Attempt</th>
              <th className="text-left py-1.5 px-3 font-normal">Due</th>
              <th className="text-left py-1.5 px-3 font-normal">Last error</th>
            </tr>
          </thead>
          <tbody>
            {rows!.map((r) => (
              <tr key={r.issue_id} className="border-b border-border-default/60">
                <td className="py-1.5 px-3 font-mono">{r.identifier || r.issue_id}</td>
                <td className="py-1.5 px-3">{r.attempt}</td>
                <td className="py-1.5 px-3 text-fg-muted">{r.due_at && relTime(r.due_at)}</td>
                <td className="py-1.5 px-3 text-red-300/80 truncate max-w-[24rem]">
                  {r.error}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
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
