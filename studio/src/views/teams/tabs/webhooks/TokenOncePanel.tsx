import { useMemo } from "react";

import {
  inboundWebhookURL,
  providerSetupSnippet,
  type WebhookConfig,
} from "@/api/webhooks";
import { Button } from "@/components/ui/Button";
import { CopyButton } from "@/components/ui/CopyButton";
import { Dialog } from "@/components/ui/Dialog";

// Shown once after a webhook is created or rotated: the inbound URL and
// the plaintext token. The token is never persisted — closing the dialog
// throws it away, so the operator must copy it on the spot.
export function TokenOncePanel({
  config,
  token,
  onClose,
}: {
  config: WebhookConfig;
  token: string;
  onClose: () => void;
}) {
  const url = useMemo(
    () => inboundWebhookURL(config.provider, config.id),
    [config.provider, config.id],
  );
  const snippet = providerSetupSnippet(config.provider, url, token);
  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title={`Token for ${config.name}`}
      description="The token is shown once. Copy it now — you will not be able to retrieve it again."
      widthClass="max-w-2xl"
      footer={
        <Button variant="primary" onClick={onClose}>
          Done — hide token
        </Button>
      }
    >
      <div className="space-y-4 text-sm" data-testid="token-once-panel">
        <section>
          <div className="text-xs uppercase tracking-wider text-fg-muted">Inbound URL</div>
          <div className="flex items-center gap-2 bg-surface-0 border border-border-subtle rounded p-2 font-mono text-xs break-all">
            <span className="flex-1">{url}</span>
            <CopyButton value={url} variant="icon" />
          </div>
        </section>

        <section>
          <div className="text-xs uppercase tracking-wider text-fg-muted">Token (one-time)</div>
          <div className="flex items-center gap-2 bg-surface-0 border border-border-subtle rounded p-2 font-mono text-xs break-all">
            <span className="flex-1" data-testid="webhook-token">
              {token}
            </span>
            <CopyButton value={token} variant="icon" />
          </div>
        </section>

        <section>
          <div className="text-xs uppercase tracking-wider text-fg-muted">{snippet.title}</div>
          <ol className="list-decimal list-inside text-xs text-fg-muted space-y-0.5 mt-1">
            {snippet.steps.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ol>
          {snippet.example && (
            <pre className="mt-2 bg-surface-0 border border-border-subtle rounded p-2 text-xs whitespace-pre-wrap break-all">
              {snippet.example}
            </pre>
          )}
        </section>
      </div>
    </Dialog>
  );
}
