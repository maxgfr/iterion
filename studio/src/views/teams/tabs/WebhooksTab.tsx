import { errorMessage } from "@/lib/errorHints";
import { useEffect, useMemo, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";

import {
  FeatureUnavailableError,
  type WebhookConfig,
  type WebhookDelivery,
  type WebhookProvider,
  createWebhook,
  deleteWebhook,
  inboundWebhookURL,
  listWebhookDeliveries,
  listWebhooks,
  providerSetupSnippet,
  rotateWebhook,
  updateWebhook,
} from "@/api/webhooks";
import { type BotEntryWithSchema, listBots } from "@/api/bots";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { CopyButton } from "@/components/ui/CopyButton";
import { Dialog } from "@/components/ui/Dialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Radio } from "@/components/ui/Radio";
import { Select } from "@/components/ui/Select";
import { TagInput } from "@/components/ui/TagInput";
import ConfirmDialog from "@/components/shared/ConfirmDialog";

interface Props {
  teamID: string;
  canManage: boolean;
}

const PROVIDERS: Array<{ id: WebhookProvider; label: string; available: boolean }> = [
  { id: "gitlab", label: "GitLab", available: true },
  { id: "github", label: "GitHub", available: true },
  { id: "forgejo", label: "Forgejo / Gitea", available: true },
  { id: "generic", label: "Generic (JSON)", available: true },
];

export default function WebhooksTab({ teamID, canManage }: Props) {
  const [webhooks, setWebhooks] = useState<WebhookConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [botsError, setBotsError] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [creating, setCreating] = useState(false);
  const [bots, setBots] = useState<BotEntryWithSchema[]>([]);
  const [issued, setIssued] = useState<{ config: WebhookConfig; token: string } | null>(
    null,
  );
  const [deliveriesFor, setDeliveriesFor] = useState<WebhookConfig | null>(null);
  const [rotateTarget, setRotateTarget] = useState<WebhookConfig | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<WebhookConfig | null>(null);

  const reload = async () => {
    setLoading(true);
    setErr(null);
    try {
      const list = await listWebhooks(teamID);
      setWebhooks(list);
      setUnavailable(false);
    } catch (e) {
      if (e instanceof FeatureUnavailableError) {
        setUnavailable(true);
      } else {
        setErr(errorMessage(e));
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
    // best-effort: load bots so the create dialog can render the picker
    void listBots()
      .then((b) => {
        setBots(b);
        setBotsError(null);
      })
      .catch((e) =>
        setBotsError((e as Error)?.message ?? "Failed to load bots."),
      );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID]);

  const toggleEnabled = async (cfg: WebhookConfig) => {
    try {
      await updateWebhook(teamID, cfg.id, { enabled: !cfg.enabled });
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    }
  };

  const doRotate = async () => {
    if (!rotateTarget) return;
    try {
      const r = await rotateWebhook(teamID, rotateTarget.id);
      setRotateTarget(null);
      setIssued(r);
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    }
  };

  const doDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteWebhook(teamID, deleteTarget.id);
      setDeleteTarget(null);
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    }
  };

  if (unavailable) {
    return (
      <EmptyState
        title="Webhooks not enabled on this server"
        message="Inbound webhooks require a multi-tenant deployment. Run iterion server in cloud mode to use them."
      />
    );
  }

  return (
    <div className="space-y-4">
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}
      {botsError && (
        <InlineBanner tone="danger" layout="inline" title="Bots unavailable">
          {botsError}
        </InlineBanner>
      )}

      <div className="flex items-center justify-between">
        <div>
          <h3 className="font-medium">Inbound webhooks</h3>
          <p className="text-xs text-fg-subtle mt-0.5">
            Long-lived tokens an external forge can present to launch a bot.
          </p>
        </div>
        {canManage && (
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            New webhook
          </Button>
        )}
      </div>

      {loading ? (
        <EmptyState message="Loading…" />
      ) : webhooks.length === 0 ? (
        <EmptyState
          message={
            canManage
              ? "No webhooks yet. Create one to give a forge access to your bots."
              : "No webhooks yet. Ask an admin to create one."
          }
        />
      ) : (
        <div className="overflow-x-auto"><table className="w-full text-sm">
          <thead className="text-xs uppercase tracking-wider text-fg-muted text-left">
            <tr>
              <th className="px-2 py-1">Name</th>
              <th className="px-2 py-1">Provider</th>
              <th className="px-2 py-1">Bots</th>
              <th className="px-2 py-1">Last4</th>
              <th className="px-2 py-1">Status</th>
              <th className="px-2 py-1 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {webhooks.map((w) => (
              <tr key={w.id} className="border-t border-border-subtle align-top">
                <td className="px-2 py-2">
                  <div className="font-medium">{w.name}</div>
                  <div className="text-[10px] text-fg-subtle font-mono break-all">
                    {w.id}
                  </div>
                </td>
                <td className="px-2 py-2">
                  <Badge variant="neutral">{w.provider}</Badge>
                </td>
                <td className="px-2 py-2 text-xs">
                  {w.wildcard_bots ? (
                    <Badge variant="warning">wildcard</Badge>
                  ) : (
                    (w.bot_ids ?? []).join(", ") || "—"
                  )}
                </td>
                <td className="px-2 py-2 text-xs font-mono text-fg-muted">
                  …{w.token_last4 || "????"}
                </td>
                <td className="px-2 py-2">
                  {canManage ? (
                    <label className="inline-flex items-center gap-1 text-xs cursor-pointer">
                      <Checkbox
                        checked={w.enabled}
                        onChange={() => toggleEnabled(w)}
                      />
                      {w.enabled ? "enabled" : "disabled"}
                    </label>
                  ) : (
                    <span className="text-xs">{w.enabled ? "enabled" : "disabled"}</span>
                  )}
                </td>
                <td className="px-2 py-2 text-right space-x-1 whitespace-nowrap">
                  <Button size="sm" variant="ghost" onClick={() => setDeliveriesFor(w)}>
                    Deliveries
                  </Button>
                  {canManage && (
                    <>
                      <Button size="sm" variant="ghost" onClick={() => setRotateTarget(w)}>
                        Rotate
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setDeleteTarget(w)}
                        className="text-danger"
                      >
                        Delete
                      </Button>
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}

      {creating && (
        <CreateWebhookDialog
          teamID={teamID}
          bots={bots}
          onClose={() => setCreating(false)}
          onCreated={(r) => {
            setCreating(false);
            setIssued(r);
            void reload();
          }}
        />
      )}

      {issued && (
        <TokenOncePanel
          config={issued.config}
          token={issued.token}
          onClose={() => setIssued(null)}
        />
      )}

      {deliveriesFor && (
        <DeliveriesDrawer
          teamID={teamID}
          webhook={deliveriesFor}
          onClose={() => setDeliveriesFor(null)}
        />
      )}

      <ConfirmDialog
        open={rotateTarget !== null}
        title={`Rotate ${rotateTarget?.name ?? ""}?`}
        message="The current token will stop working immediately. You will see the new token once — make sure to copy it before closing the panel."
        confirmLabel="Rotate"
        confirmVariant="danger"
        onConfirm={() => void doRotate()}
        onCancel={() => setRotateTarget(null)}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        title={`Delete ${deleteTarget?.name ?? ""}?`}
        message="The webhook URL will return 404 immediately and incoming events will be discarded. This cannot be undone."
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={() => void doDelete()}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}

// ---- Create dialog ----

function CreateWebhookDialog({
  teamID,
  bots,
  onClose,
  onCreated,
}: {
  teamID: string;
  bots: BotEntryWithSchema[];
  onClose: () => void;
  onCreated: (r: { config: WebhookConfig; token: string }) => void;
}) {
  const [name, setName] = useState("");
  const [provider, setProvider] = useState<WebhookProvider>("gitlab");
  const [wildcard, setWildcard] = useState(false);
  const [botIDs, setBotIDs] = useState<string[]>([]);
  const [defaultBot, setDefaultBot] = useState("");
  const [projectAllow, setProjectAllow] = useState<string[]>([]);
  const [eventAllow, setEventAllow] = useState<string[]>([]);
  const [forgeBaseURL, setForgeBaseURL] = useState("");
  const [rate, setRate] = useState<number>(1.0);
  const [burst, setBurst] = useState<number>(10);
  const [monthlyCap, setMonthlyCap] = useState<number>(0);
  const [launchVars, setLaunchVars] = useState<Array<{ k: string; v: string }>>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const lvs = launchVars
        .filter((kv) => kv.k.trim() !== "")
        .reduce<Record<string, string>>((acc, kv) => {
          acc[kv.k.trim()] = kv.v;
          return acc;
        }, {});
      const r = await createWebhook(teamID, {
        name: name.trim(),
        provider,
        wildcard_bots: wildcard,
        bot_ids: wildcard ? undefined : botIDs,
        default_bot_id: defaultBot || undefined,
        project_allowlist: projectAllow.length ? projectAllow : undefined,
        event_allowlist: eventAllow.length ? eventAllow : undefined,
        forge_base_url: forgeBaseURL.trim() || undefined,
        rate_limit: { rate, burst },
        monthly_call_limit: monthlyCap > 0 ? monthlyCap : undefined,
        launch_vars: Object.keys(lvs).length ? lvs : undefined,
      });
      onCreated(r);
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const valid = name.trim() !== "" && (wildcard || botIDs.length > 0);

  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title="New inbound webhook"
      widthClass="max-w-2xl"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={() => void submit()} loading={busy} disabled={!valid}>
            Create webhook
          </Button>
        </>
      }
    >
      {err && (
        <InlineBanner tone="danger" layout="inline" className="mb-3">
          {err}
        </InlineBanner>
      )}
      <div className="space-y-3 text-sm">
        <Field label="Name">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="GitLab — review bots"
            required
          />
        </Field>

        <Field label="Provider">
          <div className="flex flex-wrap gap-2">
            {PROVIDERS.map((p) => (
              <label
                key={p.id}
                className={`inline-flex items-center gap-1.5 border rounded px-2 py-1 text-xs cursor-pointer ${
                  provider === p.id
                    ? "border-accent bg-accent-soft"
                    : "border-border-subtle"
                } ${p.available ? "" : "opacity-60 cursor-not-allowed"}`}
              >
                <Radio
                  name="provider"
                  value={p.id}
                  checked={provider === p.id}
                  disabled={!p.available}
                  onChange={() => p.available && setProvider(p.id)}
                />
                {p.label}
              </label>
            ))}
          </div>
        </Field>

        {provider === "gitlab" && (
          <Field label="Forge base URL (optional)">
            <Input
              value={forgeBaseURL}
              onChange={(e) => setForgeBaseURL(e.target.value)}
              placeholder="https://gitlab.example.com"
            />
            <div className="text-xs text-fg-subtle mt-1">
              Pin the GitLab instance this webhook may send its bot token to. A
              delivery whose merge-request URL host differs is refused. Leave
              empty to derive the host from the payload.
            </div>
          </Field>
        )}

        <Field label="Bot scope">
          <div className="space-y-2">
            <label className="inline-flex items-center gap-2 text-xs">
              <Checkbox
                checked={wildcard}
                onChange={(e) => setWildcard(e.target.checked)}
              />
              Allow any bot (wildcard — broad surface, audited as such)
            </label>
            {!wildcard && (
              <div className="space-y-1">
                <div className="text-xs text-fg-subtle">Pick the bots this webhook can launch:</div>
                <div className="flex flex-wrap gap-1 max-h-32 overflow-auto border border-border-subtle rounded p-2">
                  {bots.length === 0 ? (
                    <span className="text-xs text-fg-subtle">No bots discovered.</span>
                  ) : (
                    bots.map((b) => {
                      const checked = botIDs.includes(b.name);
                      return (
                        <label
                          key={b.name}
                          className={`inline-flex items-center gap-1 border rounded px-2 py-0.5 text-xs cursor-pointer ${
                            checked ? "border-accent bg-accent-soft" : "border-border-subtle"
                          }`}
                        >
                          <Checkbox
                            checked={checked}
                            onChange={() => {
                              if (checked) setBotIDs(botIDs.filter((x) => x !== b.name));
                              else setBotIDs([...botIDs, b.name]);
                            }}
                          />
                          <span>{b.display_name || b.name}</span>
                        </label>
                      );
                    })
                  )}
                </div>
                {botIDs.length > 1 && (
                  <Field label="Default bot (optional)" inline>
                    <Select
                      value={defaultBot}
                      onChange={(e) => setDefaultBot(e.target.value)}
                    >
                      <option value="">— pick at delivery time —</option>
                      {botIDs.map((id) => (
                        <option key={id} value={id}>
                          {id}
                        </option>
                      ))}
                    </Select>
                  </Field>
                )}
              </div>
            )}
          </div>
        </Field>

        <Field label="Project allowlist (paths)">
          <TagInput
            value={projectAllow}
            onChange={setProjectAllow}
            placeholder="group/repo"
          />
        </Field>

        <Field label="Event allowlist">
          <TagInput
            value={eventAllow}
            onChange={setEventAllow}
            placeholder="merge_request, note, …"
          />
        </Field>

        <div className="grid grid-cols-3 gap-2">
          <Field label="Rate (req/s)">
            <Input
              type="number"
              min={0}
              step={0.1}
              value={String(rate)}
              onChange={(e) => setRate(Number(e.target.value))}
            />
          </Field>
          <Field label="Burst">
            <Input
              type="number"
              min={0}
              value={String(burst)}
              onChange={(e) => setBurst(Number(e.target.value))}
            />
          </Field>
          <Field label="Monthly cap (0 = inherit)">
            <Input
              type="number"
              min={0}
              value={String(monthlyCap)}
              onChange={(e) => setMonthlyCap(Number(e.target.value))}
            />
          </Field>
        </div>

        <Field label="Launch vars">
          <div className="space-y-1">
            {launchVars.map((kv, i) => (
              <div key={i} className="flex gap-1">
                <Input
                  placeholder="key"
                  value={kv.k}
                  onChange={(e) => {
                    const next = [...launchVars];
                    next[i] = { ...kv, k: e.target.value };
                    setLaunchVars(next);
                  }}
                />
                <Input
                  placeholder="value"
                  value={kv.v}
                  onChange={(e) => {
                    const next = [...launchVars];
                    next[i] = { ...kv, v: e.target.value };
                    setLaunchVars(next);
                  }}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setLaunchVars(launchVars.filter((_, j) => j !== i))}
                >
                  ×
                </Button>
              </div>
            ))}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setLaunchVars([...launchVars, { k: "", v: "" }])}
            >
              + Add var
            </Button>
          </div>
        </Field>
      </div>
    </Dialog>
  );
}

// ---- Token-once panel ----

function TokenOncePanel({
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

// ---- Deliveries drawer ----

function DeliveriesDrawer({
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
        <div className="overflow-x-auto"><table className="w-full text-xs">
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
        </table></div>
      )}
    </Dialog>
  );
}

function DeliveryStatusBadge({ status }: { status: string }) {
  const map: Record<string, "success" | "warning" | "danger" | "neutral"> = {
    launched: "success",
    accepted: "success",
    duplicate: "neutral",
    rate_limited: "warning",
    quota_exceeded: "warning",
    filtered: "neutral",
    invalid: "danger",
    launch_error: "danger",
  };
  const variant = map[status] ?? "neutral";
  return <Badge variant={variant}>{status}</Badge>;
}

// ---- Tiny field helper (local to the file) ----

function Field({
  label,
  children,
  inline,
}: {
  label: string;
  children: React.ReactNode;
  inline?: boolean;
}) {
  return (
    <label className={inline ? "flex items-center gap-2 text-xs" : "block text-xs space-y-1"}>
      <span className="text-fg-muted">{label}</span>
      <div className={inline ? "" : "block"}>{children}</div>
    </label>
  );
}
