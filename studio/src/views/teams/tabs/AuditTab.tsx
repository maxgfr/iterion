import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";

import {
  type AuditEvent,
  type AuditQuery,
  FeatureUnavailableError,
  listTeamAudit,
} from "@/api/audit";

import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";

interface Props {
  teamID: string;
  canManage: boolean;
}

const PAGE = 50;

export default function AuditTab({ teamID, canManage }: Props) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [filter, setFilter] = useState<AuditQuery>({});
  const [nextOffset, setNextOffset] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  const fetchPage = async (q: AuditQuery, append: boolean) => {
    setLoading(true);
    setErr(null);
    try {
      const r = await listTeamAudit(teamID, { ...q, limit: PAGE });
      setEvents(append ? [...events, ...r.events] : r.events);
      // When the server returns a full page assume there may be more.
      // The Go handler always echoes offset + len(events), so we treat
      // anything < requested page size as exhausted.
      setNextOffset(r.events.length < PAGE ? null : r.next_offset);
    } catch (e) {
      if (e instanceof FeatureUnavailableError) setUnavailable(true);
      else setErr(errorMessage(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!canManage) return;
    void fetchPage({ ...filter, offset: 0 }, false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID, canManage]);

  if (!canManage) {
    return (
      <EmptyState
        title="Audit log is admin-only"
        message="Audit rows expose actor emails and IPs across the whole org. Only team admins can read them."
      />
    );
  }
  if (unavailable) {
    return (
      <EmptyState
        title="Audit log not enabled on this server"
        message="The audit log requires the cloud-mode audit store."
      />
    );
  }

  const apply = () => void fetchPage({ ...filter, offset: 0 }, false);
  const loadMore = () => {
    if (nextOffset == null) return;
    void fetchPage({ ...filter, offset: nextOffset }, true);
  };

  return (
    <div className="space-y-3">
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-4 gap-2 text-sm">
        <Input
          placeholder="Action (e.g. webhook.rotated)"
          value={filter.action ?? ""}
          onChange={(e) => setFilter({ ...filter, action: e.target.value })}
        />
        <Input
          placeholder="Actor id"
          value={filter.actor ?? ""}
          onChange={(e) => setFilter({ ...filter, actor: e.target.value })}
        />
        <Input
          type="datetime-local"
          value={filter.from ?? ""}
          onChange={(e) => setFilter({ ...filter, from: e.target.value })}
        />
        <Input
          type="datetime-local"
          value={filter.to ?? ""}
          onChange={(e) => setFilter({ ...filter, to: e.target.value })}
        />
      </div>
      <div className="flex gap-2">
        <Button size="sm" variant="primary" onClick={apply} loading={loading}>
          Apply filters
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            setFilter({});
            void fetchPage({}, false);
          }}
        >
          Clear
        </Button>
      </div>

      {events.length === 0 && !loading ? (
        <EmptyState message="No events for this filter." />
      ) : (
        <div className="overflow-x-auto"><table className="w-full text-sm">
          <thead className="text-xs uppercase tracking-wider text-fg-muted text-left">
            <tr>
              <th className="px-2 py-1">When</th>
              <th className="px-2 py-1">Actor</th>
              <th className="px-2 py-1">Action</th>
              <th className="px-2 py-1">Target</th>
              <th className="px-2 py-1">IP</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={e.id} className="border-t border-border-subtle align-top">
                <td className="px-2 py-2 text-fg-muted whitespace-nowrap">
                  {new Date(e.created_at).toLocaleString()}
                </td>
                <td className="px-2 py-2 text-xs">
                  <div>{e.actor_id ?? "—"}</div>
                  <div className="text-fg-subtle">{e.actor_kind ?? ""}</div>
                </td>
                <td className="px-2 py-2 font-mono text-xs">{e.action}</td>
                <td className="px-2 py-2 text-xs">
                  <div>{e.target ?? ""}</div>
                  <div className="text-fg-subtle font-mono break-all">{e.target_id ?? ""}</div>
                  {e.meta && (
                    <details className="text-fg-muted">
                      <summary className="cursor-pointer text-[10px]">meta</summary>
                      <pre className="whitespace-pre-wrap break-all text-[10px]">
                        {JSON.stringify(e.meta, null, 2)}
                      </pre>
                    </details>
                  )}
                </td>
                <td className="px-2 py-2 text-xs font-mono text-fg-muted">{e.ip ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}

      {nextOffset != null && (
        <div className="flex justify-center">
          <Button size="sm" variant="ghost" loading={loading} onClick={loadMore}>
            Load more
          </Button>
        </div>
      )}
    </div>
  );
}
