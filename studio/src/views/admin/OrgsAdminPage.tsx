import { useCallback, useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useAuth } from "@/auth/AuthContext";
import {
  type OrgView,
  createOrg,
  fmtQuotaGiB,
  gibToBytes,
  listOrgs,
  setOrgStatus,
  updateOrg,
} from "@/api/orgs";
import { FeatureUnavailableError, getAdminOrgUsage, type OrgUsage, fmtBytes, fmtUSD, pct } from "@/api/usage";

import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useHeaderSlot } from "@/components/shared/useHeaderSlot";

export default function OrgsAdminPage() {
  const { user } = useAuth();
  const isSuper = user?.is_super_admin ?? false;

  const [orgs, setOrgs] = useState<OrgView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [active, setActive] = useState<OrgView | null>(null);

  useHeaderSlot({
    left: <span className="text-sm font-semibold">Organizations</span>,
    right: <span className="text-xs text-fg-muted">{orgs.length} org(s)</span>,
  });

  const refresh = useCallback(async () => {
    try {
      setOrgs(await listOrgs());
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    if (isSuper) void refresh();
  }, [isSuper, refresh]);

  if (!isSuper) {
    return (
      <div className="p-6">
        <p className="text-sm text-fg-muted">Super-admin only.</p>
      </div>
    );
  }

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const create = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    void run(async () => {
      await createOrg({ name: name.trim(), owner_email: ownerEmail.trim() || undefined });
      setName("");
      setOwnerEmail("");
    });
  };

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-5xl mx-auto p-3 sm:p-6 space-y-4">
        {error && (
          <InlineBanner tone="danger" layout="inline">
            {error}
          </InlineBanner>
        )}

        <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
          <h3 className="font-medium">Create an organization</h3>
          <form onSubmit={create} className="flex flex-wrap gap-2">
            <input
              className="flex-1 min-w-[160px] bg-surface-0 border border-border-subtle rounded px-3 py-2"
              placeholder="Org name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
            <input
              type="email"
              className="flex-1 min-w-[200px] bg-surface-0 border border-border-subtle rounded px-3 py-2"
              placeholder="owner email (optional)"
              value={ownerEmail}
              onChange={(e) => setOwnerEmail(e.target.value)}
            />
            <button
              type="submit"
              disabled={busy}
              className="bg-fg-accent text-surface-0 rounded px-3 py-2 text-sm disabled:opacity-50"
            >
              Create
            </button>
          </form>
        </section>

        <section className="bg-surface-1 border border-border-subtle rounded overflow-hidden">
          <table className="w-full text-sm">
            <thead className="text-left text-fg-muted border-b border-border-subtle">
              <tr>
                <th className="px-3 py-2 font-medium">Name</th>
                <th className="px-3 py-2 font-medium">Slug</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Memory quota</th>
                <th className="px-3 py-2 font-medium text-right">Manage</th>
              </tr>
            </thead>
            <tbody>
              {orgs.map((o) => (
                <tr
                  key={o.id}
                  className="border-b border-border-subtle last:border-0 cursor-pointer hover:bg-surface-2"
                  onClick={() => setActive(o)}
                >
                  <td className="px-3 py-2">
                    {o.name}
                    {o.personal && <span className="ml-2 text-xs text-fg-muted">personal</span>}
                  </td>
                  <td className="px-3 py-2 text-fg-muted">{o.slug}</td>
                  <td className="px-3 py-2">
                    <span
                      className={
                        o.status === "suspended"
                          ? "text-danger"
                          : o.status === "read_only"
                            ? "text-fg-muted"
                            : "text-fg-success"
                      }
                    >
                      {o.status}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-fg-muted">{fmtQuotaGiB(o.memory_quota_bytes)}</td>
                  <td className="px-3 py-2 text-right">
                    <Button size="sm" variant="ghost" onClick={(e) => { e.stopPropagation(); setActive(o); }}>
                      Open
                    </Button>
                  </td>
                </tr>
              ))}
              {orgs.length === 0 && (
                <tr>
                  <td className="px-3 py-6 text-center text-fg-muted" colSpan={5}>
                    No organizations yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </section>
      </div>

      {active && (
        <OrgDrawer
          org={active}
          busy={busy}
          onClose={() => setActive(null)}
          onAfterUpdate={refresh}
          run={run}
        />
      )}
    </div>
  );
}

function OrgDrawer({
  org,
  busy,
  onClose,
  onAfterUpdate,
  run,
}: {
  org: OrgView;
  busy: boolean;
  onClose: () => void;
  onAfterUpdate: () => Promise<void>;
  run: (fn: () => Promise<unknown>) => Promise<void>;
}) {
  const [usage, setUsage] = useState<OrgUsage | null>(null);
  const [usageErr, setUsageErr] = useState<string | null>(null);

  // Quota draft state — initialised from org.
  const initialGiB = org.memory_quota_bytes ? org.memory_quota_bytes / (1 << 30) : 0;
  const [memGiB, setMemGiB] = useState<number>(initialGiB);
  const [monthlyRuns, setMonthlyRuns] = useState<number>(org.monthly_run_quota ?? 0);
  const [costCap, setCostCap] = useState<number>(org.monthly_cost_cap_usd ?? 0);
  const [maxConcurrent, setMaxConcurrent] = useState<number>(org.max_concurrent_runs ?? 0);
  const [launchRate, setLaunchRate] = useState<number>(org.launch_rate_per_min ?? 0);

  // Status draft.
  const [statusDraft, setStatusDraft] = useState<string>(org.status);
  const [reason, setReason] = useState("");

  useEffect(() => {
    let alive = true;
    setUsageErr(null);
    getAdminOrgUsage(org.id)
      .then((u) => {
        if (alive) setUsage(u);
      })
      .catch((e) => {
        if (!alive) return;
        if (e instanceof FeatureUnavailableError) setUsageErr("Usage view not enabled.");
        else setUsageErr((e as Error).message);
      });
    return () => {
      alive = false;
    };
  }, [org.id]);

  const saveQuotas = () =>
    run(async () => {
      await updateOrg(org.id, {
        memory_quota_bytes: memGiB > 0 ? gibToBytes(memGiB) : 0,
        monthly_run_quota: monthlyRuns,
        monthly_cost_cap_usd: costCap,
        max_concurrent_runs: maxConcurrent,
        launch_rate_per_min: launchRate,
      });
      await onAfterUpdate();
    });

  const saveStatus = () =>
    run(async () => {
      await setOrgStatus(org.id, statusDraft, reason.trim() || undefined);
      await onAfterUpdate();
    });

  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title={org.name}
      description={
        <span>
          <span className="font-mono text-xs">{org.id}</span>
          {org.personal ? " · personal" : ""}
        </span>
      }
      widthClass="max-w-3xl"
      footer={
        <Button variant="secondary" onClick={onClose}>
          Close
        </Button>
      }
    >
      {usageErr && (
        <div className="text-sm text-fg-muted bg-warning-soft border border-warning/40 rounded px-3 py-2 mb-3">
          {usageErr}
        </div>
      )}

      <section className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs mb-4">
        <Stat title="Members" value={String(usage?.members ?? "—")} />
        <Stat
          title="Memory"
          value={
            usage
              ? `${fmtBytes(usage.memory_used_bytes)} / ${fmtBytes(usage.effective_memory_quota_bytes)}`
              : "—"
          }
          progress={usage ? pct(usage.memory_used_bytes, usage.effective_memory_quota_bytes) : null}
        />
        <Stat
          title="Runs this month"
          value={
            usage
              ? `${usage.runs_this_month}${usage.monthly_run_quota > 0 ? ` / ${usage.monthly_run_quota}` : ""}`
              : "—"
          }
          progress={usage ? pct(usage.runs_this_month, usage.monthly_run_quota) : null}
        />
        <Stat
          title="Cost this month"
          value={
            usage
              ? `${fmtUSD(usage.cost_usd_this_month)}${
                  usage.monthly_cost_cap_usd && usage.monthly_cost_cap_usd > 0
                    ? ` / ${fmtUSD(usage.monthly_cost_cap_usd)}`
                    : ""
                }`
              : "—"
          }
          progress={
            usage && usage.monthly_cost_cap_usd
              ? pct(usage.cost_usd_this_month, usage.monthly_cost_cap_usd)
              : null
          }
        />
        <Stat title="API keys" value={String(usage?.api_key_count ?? "—")} />
        <Stat title="Secrets" value={String(usage?.generic_secret_count ?? "—")} />
        <Stat title="Bindings" value={String(usage?.bot_binding_count ?? "—")} />
        <Stat title="Webhooks" value={String(usage?.webhook_count ?? "—")} />
      </section>

      <section className="space-y-3 mb-6">
        <h4 className="font-medium">Quotas</h4>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <Field label="Memory quota (GiB, 0 = default)">
            <Input
              type="number"
              min={0}
              step={0.5}
              value={String(memGiB)}
              onChange={(e) => setMemGiB(Number(e.target.value))}
            />
          </Field>
          <Field label="Monthly run quota (0 = unlimited)">
            <Input
              type="number"
              min={0}
              value={String(monthlyRuns)}
              onChange={(e) => setMonthlyRuns(Number(e.target.value))}
            />
          </Field>
          <Field label="Monthly cost cap USD (0 = unlimited)">
            <Input
              type="number"
              min={0}
              step={1}
              value={String(costCap)}
              onChange={(e) => setCostCap(Number(e.target.value))}
            />
          </Field>
          <Field label="Max concurrent runs (0 = unlimited)">
            <Input
              type="number"
              min={0}
              value={String(maxConcurrent)}
              onChange={(e) => setMaxConcurrent(Number(e.target.value))}
            />
          </Field>
          <Field label="Launch rate / min (0 = unlimited)">
            <Input
              type="number"
              min={0}
              value={String(launchRate)}
              onChange={(e) => setLaunchRate(Number(e.target.value))}
            />
          </Field>
        </div>
        <Button variant="primary" loading={busy} onClick={() => void saveQuotas()}>
          Save quotas
        </Button>
      </section>

      <section className="space-y-3">
        <h4 className="font-medium">Status</h4>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <Field label="Status">
            <Select value={statusDraft} onChange={(e) => setStatusDraft(e.target.value)}>
              <option value="active">active</option>
              <option value="suspended">suspended</option>
              <option value="read_only">read_only</option>
            </Select>
          </Field>
          <Field label="Reason (audit log)">
            <Input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="optional"
            />
          </Field>
        </div>
        <Button
          variant={statusDraft === "suspended" ? "danger" : "primary"}
          loading={busy}
          onClick={() => void saveStatus()}
        >
          Apply status
        </Button>
      </section>
    </Dialog>
  );
}

function Stat({
  title,
  value,
  progress,
}: {
  title: string;
  value: string;
  progress?: number | null;
}) {
  return (
    <div className="bg-surface-0 border border-border-subtle rounded p-2">
      <div className="text-fg-muted">{title}</div>
      <div className="font-medium">{value}</div>
      {progress != null && (
        <div className="mt-1 h-1 bg-surface-2 rounded overflow-hidden">
          <div
            className={`h-full ${progress > 90 ? "bg-danger" : progress > 70 ? "bg-warning" : "bg-accent"}`}
            style={{ width: `${progress}%` }}
          />
        </div>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-xs space-y-1">
      <span className="text-fg-muted">{label}</span>
      <div>{children}</div>
    </label>
  );
}
