import { useCallback, useEffect, useState } from "react";
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
import { useHeaderSlot } from "@/components/shared/useHeaderSlot";

export default function OrgsAdminPage() {
  const { user } = useAuth();
  const isSuper = user?.is_super_admin ?? false;

  const [orgs, setOrgs] = useState<OrgView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");

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

  const toggleSuspend = (o: OrgView) => {
    const next = o.status === "suspended" ? "active" : "suspended";
    void run(() => setOrgStatus(o.id, next, next === "suspended" ? "admin console" : ""));
  };

  const editMemoryQuota = (o: OrgView) => {
    const cur = o.memory_quota_bytes ? o.memory_quota_bytes / (1 << 30) : 0;
    const input = window.prompt(`Memory quota for ${o.name} in GiB (0 = default 1 GiB):`, String(cur));
    if (input == null) return;
    const gib = Number(input);
    if (Number.isNaN(gib) || gib < 0) {
      setError("invalid quota");
      return;
    }
    void run(() => updateOrg(o.id, { memory_quota_bytes: gibToBytes(gib) }));
  };

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-4xl mx-auto p-3 sm:p-6 space-y-4">
        {error && (
          <div className="text-sm text-fg-error bg-surface-warn-subtle border border-border-warn rounded px-3 py-2">
            {error}
          </div>
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
                <th className="px-3 py-2 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {orgs.map((o) => (
                <tr key={o.id} className="border-b border-border-subtle last:border-0">
                  <td className="px-3 py-2">
                    {o.name}
                    {o.personal && <span className="ml-2 text-xs text-fg-muted">personal</span>}
                  </td>
                  <td className="px-3 py-2 text-fg-muted">{o.slug}</td>
                  <td className="px-3 py-2">
                    <span
                      className={
                        o.status === "suspended"
                          ? "text-fg-error"
                          : o.status === "read_only"
                            ? "text-fg-muted"
                            : "text-fg-success"
                      }
                    >
                      {o.status}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-fg-muted">{fmtQuotaGiB(o.memory_quota_bytes)}</td>
                  <td className="px-3 py-2 text-right space-x-2 whitespace-nowrap">
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => editMemoryQuota(o)}
                      className="text-xs border border-border-subtle rounded px-2 py-1 disabled:opacity-50"
                    >
                      Quota
                    </button>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => toggleSuspend(o)}
                      className="text-xs border border-border-subtle rounded px-2 py-1 disabled:opacity-50"
                    >
                      {o.status === "suspended" ? "Restore" : "Suspend"}
                    </button>
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
    </div>
  );
}
