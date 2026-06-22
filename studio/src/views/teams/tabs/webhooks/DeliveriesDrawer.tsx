import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";

import {
  type WebhookConfig,
  type WebhookDelivery,
  listWebhookDeliveries,
} from "@/api/webhooks";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { InlineBanner } from "@/components/ui/InlineBanner";

import { DeliveryStatusBadge } from "./DeliveryStatusBadge";

// Side drawer that lists recent deliveries for one webhook. Refresh is
// manual (operator-triggered) — the spine already audits each delivery,
// this is a read-only window over that log.
export function DeliveriesDrawer({
  teamID,
  webhook,
  onClose,
}: {
  teamID: string;
  webhook: WebhookConfig;
  onClose: () => void;
}) {
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setErr(null);
    try {
      setDeliveries(await listWebhookDeliveries(teamID, webhook.id));
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID, webhook.id]);

  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title={`Deliveries — ${webhook.name}`}
      widthClass="max-w-3xl"
      footer={
        <>
          <Button variant="secondary" onClick={() => void load()}>
            Refresh
          </Button>
          <Button variant="primary" onClick={onClose}>
            Close
          </Button>
        </>
      }
    >
      {err && (
        <InlineBanner tone="danger" layout="inline" className="mb-3">
          {err}
        </InlineBanner>
      )}
      {loading ? (
        <EmptyState message="Loading…" />
      ) : deliveries.length === 0 ? (
        <EmptyState message="No deliveries yet. Push an event from the forge to see it appear here." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead className="text-fg-muted text-left">
              <tr>
                <th className="px-2 py-1">Status</th>
                <th className="px-2 py-1">Received</th>
                <th className="px-2 py-1">Event</th>
                <th className="px-2 py-1">From</th>
                <th className="px-2 py-1">Error</th>
              </tr>
            </thead>
            <tbody>
              {deliveries.map((d) => (
                <tr key={d.id} className="border-t border-border-subtle">
                  <td className="px-2 py-2">
                    <DeliveryStatusBadge status={d.status} />
                  </td>
                  <td className="px-2 py-2 text-fg-muted whitespace-nowrap">
                    {new Date(d.received_at).toLocaleString()}
                  </td>
                  <td className="px-2 py-2">
                    {d.event_kind ?? "—"}
                    {d.event_action ? ` / ${d.event_action}` : ""}
                  </td>
                  <td className="px-2 py-2 text-fg-muted">{d.source_ip ?? "—"}</td>
                  <td className="px-2 py-2 text-danger">{d.error ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Dialog>
  );
}
