import { useState } from "react";

import {
  type WebhookConfig,
  type WebhookProvider,
  createWebhook,
} from "@/api/webhooks";
import type { BotEntryWithSchema } from "@/api/bots";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Dialog } from "@/components/ui/Dialog";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Input } from "@/components/ui/Input";
import { Radio } from "@/components/ui/Radio";
import { Select } from "@/components/ui/Select";
import { TagInput } from "@/components/ui/TagInput";
import { useAsyncAction } from "@/hooks/useAsyncAction";

import { Field } from "./Field";

const PROVIDERS: Array<{ id: WebhookProvider; label: string; available: boolean }> = [
  { id: "gitlab", label: "GitLab", available: true },
  { id: "github", label: "GitHub", available: true },
  { id: "forgejo", label: "Forgejo / Gitea", available: true },
  { id: "generic", label: "Generic (JSON)", available: true },
];

// Modal form that POSTs a new inbound webhook. The parent receives the
// freshly-minted (config, token) pair so it can hand it off to the
// token-once panel.
export function CreateWebhookDialog({
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
  const { busy, error: err, run } = useAsyncAction();

  const submit = () =>
    run(async () => {
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
    });

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
