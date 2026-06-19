import { useEffect, useMemo, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useLocation, useParams, useSearch } from "wouter";
import { useConfirm } from "@/hooks/useConfirm";
import { useAuth } from "@/auth/AuthContext";
import {
  type InvitationView,
  type TeamMemberView,
  createInvitation,
  deleteInvitation,
  listInvitations,
  listTeamMembers,
  removeMember,
  updateMemberRole,
} from "@/api/byok";
import ApiKeysPanel from "@/views/settings/ApiKeys";
import { useHeaderSlot } from "@/components/shared/useHeaderSlot";

import IntegrationsTab from "./tabs/IntegrationsTab";
import WebhooksTab from "./tabs/WebhooksTab";
import SecretsTab from "./tabs/SecretsTab";
import BindingsTab from "./tabs/BindingsTab";
import UsageTab from "./tabs/UsageTab";
import AuditTab from "./tabs/AuditTab";
import MemoryTab from "./tabs/MemoryTab";

const ROLES = ["viewer", "member", "admin", "owner"] as const;

type Tab =
  | "members"
  | "api-keys"
  | "integrations"
  | "webhooks"
  | "secrets"
  | "bindings"
  | "usage"
  | "audit"
  | "memory";

const TABS: Array<{ id: Tab; label: string }> = [
  { id: "members", label: "Members + invitations" },
  { id: "api-keys", label: "API keys" },
  { id: "integrations", label: "Integrations" },
  { id: "webhooks", label: "Webhooks" },
  { id: "secrets", label: "Secrets" },
  { id: "bindings", label: "Bot bindings" },
  { id: "usage", label: "Usage" },
  { id: "audit", label: "Audit log" },
  { id: "memory", label: "Memory" },
];

export default function TeamPage() {
  const params = useParams<{ id: string }>();
  const teamID = params.id;
  const { teams, activeRole, user } = useAuth();
  const team = useMemo(() => teams.find((t) => t.team_id === teamID), [teams, teamID]);
  const search = useSearch();
  const [, navigate] = useLocation();
  const tabFromURL = (s: string): Tab => {
    const t = new URLSearchParams(s).get("tab");
    return TABS.some((x) => x.id === t) ? (t as Tab) : "members";
  };
  const [tab, setTab] = useState<Tab>(() => tabFromURL(search));
  // Keep the tab in sync with ?tab= so a deep link (e.g. the sidebar's
  // Integrations entry) selects the right tab even when TeamPage is already
  // mounted; selectTab writes it back so the URL stays shareable + the nav
  // highlight follows.
  useEffect(() => {
    const t = tabFromURL(search);
    setTab((cur) => (cur === t ? cur : t));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);
  const selectTab = (t: Tab) => {
    setTab(t);
    navigate(`/teams/${teamID}?tab=${t}`, { replace: true });
  };

  const canManage =
    activeRole === "admin" || activeRole === "owner" || (user?.is_super_admin ?? false);

  useHeaderSlot({
    left: team ? (
      <span className="text-sm font-semibold">
        {team.team_name}
        <span className="ml-2 text-xs text-fg-muted font-normal">/{team.team_slug}</span>
      </span>
    ) : (
      <span className="text-sm font-semibold">Team not found</span>
    ),
    right: team ? (
      <span className="text-xs text-fg-muted">Your role: {activeRole ?? "—"}</span>
    ) : null,
  });

  if (!team) {
    return (
      <div className="p-6">
        <p className="text-sm text-fg-muted">You are not a member of this team.</p>
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-6xl mx-auto p-3 sm:p-6 grid grid-cols-1 sm:grid-cols-[200px,1fr] gap-4 sm:gap-6">
        <nav className="flex sm:block sm:space-y-1 gap-1 flex-wrap">
          {TABS.map((t) => (
            <button
              key={t.id}
              onClick={() => selectTab(t.id)}
              className={`sm:w-full text-left px-3 py-2 rounded text-sm min-h-[44px] sm:min-h-0 ${
                tab === t.id ? "bg-surface-2" : "hover:bg-surface-1"
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>

        <main>
          {tab === "members" && <Members teamID={team.team_id} canManage={canManage} />}
          {tab === "api-keys" && (
            <ApiKeysPanel team={{ id: team.team_id, name: team.team_name }} />
          )}
          {tab === "integrations" && (
            <IntegrationsTab teamID={team.team_id} canManage={canManage} />
          )}
          {tab === "webhooks" && (
            <WebhooksTab teamID={team.team_id} canManage={canManage} />
          )}
          {tab === "secrets" && (
            <SecretsTab teamID={team.team_id} canManage={canManage} />
          )}
          {tab === "bindings" && (
            <BindingsTab teamID={team.team_id} canManage={canManage} />
          )}
          {tab === "usage" && <UsageTab teamID={team.team_id} />}
          {tab === "audit" && <AuditTab teamID={team.team_id} canManage={canManage} />}
          {tab === "memory" && <MemoryTab teamID={team.team_id} />}
        </main>
      </div>
    </div>
  );
}

function Members({ teamID, canManage }: { teamID: string; canManage: boolean }) {
  const [members, setMembers] = useState<TeamMemberView[]>([]);
  const [invs, setInvs] = useState<InvitationView[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState({ email: "", role: "member" });
  const [issuedToken, setIssuedToken] = useState<string | null>(null);
  const { confirm, dialog } = useConfirm();

  const reload = async () => {
    setErr(null);
    try {
      const [m, i] = await Promise.all([listTeamMembers(teamID), listInvitations(teamID)]);
      setMembers(m);
      setInvs(i);
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID]);

  const invite = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const r = await createInvitation(teamID, draft);
      setIssuedToken(r.token);
      setDraft({ email: "", role: "member" });
      void reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const cancel = async (id: string) => {
    const ok = await confirm({
      title: "Cancel invitation?",
      message: "Cancel this invitation?",
      confirmLabel: "Cancel invitation",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteInvitation(teamID, id);
      void reload();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const setRole = async (userID: string, role: string) => {
    try {
      await updateMemberRole(teamID, userID, role);
      void reload();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const kick = async (userID: string) => {
    const ok = await confirm({
      title: "Remove member?",
      message: "Remove this member from the team?",
      confirmLabel: "Remove member",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await removeMember(teamID, userID);
      void reload();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  return (
    <div className="space-y-6">
      {dialog}
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}

      {canManage && (
        <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
          <h3 className="font-medium">Invite a member</h3>
          <form onSubmit={invite} className="flex gap-2">
            <input
              type="email"
              className="flex-1 bg-surface-0 border border-border-subtle rounded px-3 py-2"
              placeholder="email@example.com"
              value={draft.email}
              onChange={(e) => setDraft({ ...draft, email: e.target.value })}
              required
            />
            <select
              className="bg-surface-0 border border-border-subtle rounded px-3 py-2"
              value={draft.role}
              onChange={(e) => setDraft({ ...draft, role: e.target.value })}
            >
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
            <button
              type="submit"
              disabled={busy}
              className="bg-accent text-fg-onAccent rounded px-3 py-2 text-sm disabled:opacity-50"
            >
              Send invite
            </button>
          </form>
          {issuedToken && (
            <div className="text-xs bg-surface-0 border border-border-subtle rounded p-3 font-mono break-all">
              Invitation token (copy + email this — it appears once):
              <br />
              {issuedToken}
              <div className="mt-2 flex gap-2 font-sans">
                <button
                  type="button"
                  onClick={() => {
                    void navigator.clipboard?.writeText(issuedToken);
                  }}
                  className="text-fg-accent hover:underline"
                >
                  Copy
                </button>
                <button
                  type="button"
                  onClick={() => setIssuedToken(null)}
                  className="text-fg-muted hover:text-fg-default"
                >
                  Done — hide
                </button>
              </div>
            </div>
          )}
        </section>
      )}

      <section>
        <h3 className="font-medium mb-2">Members</h3>
        <div className="overflow-x-auto"><table className="w-full text-sm">
          <thead className="text-xs uppercase tracking-wider text-fg-muted text-left">
            <tr>
              <th className="px-2 py-1">Email</th>
              <th className="px-2 py-1">Name</th>
              <th className="px-2 py-1">Role</th>
              <th className="px-2 py-1"></th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.user_id} className="border-t border-border-subtle">
                <td className="px-2 py-2">{m.email ?? m.user_id}</td>
                <td className="px-2 py-2">{m.name ?? "—"}</td>
                <td className="px-2 py-2">
                  {canManage ? (
                    <select
                      value={m.role}
                      onChange={(e) => setRole(m.user_id, e.target.value)}
                      className="bg-surface-0 border border-border-subtle rounded px-2 py-1 text-xs"
                    >
                      {ROLES.map((r) => (
                        <option key={r} value={r}>
                          {r}
                        </option>
                      ))}
                    </select>
                  ) : (
                    m.role
                  )}
                </td>
                <td className="px-2 py-2 text-right">
                  {canManage && (
                    <button onClick={() => kick(m.user_id)} className="text-danger hover:underline text-xs">
                      Remove
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table></div>
      </section>

      <section>
        <h3 className="font-medium mb-2">Pending invitations</h3>
        {invs.length === 0 ? (
          <div className="text-fg-muted text-sm">None.</div>
        ) : (
          <div className="overflow-x-auto"><table className="w-full text-sm">
            <thead className="text-xs uppercase tracking-wider text-fg-muted text-left">
              <tr>
                <th className="px-2 py-1">Email</th>
                <th className="px-2 py-1">Role</th>
                <th className="px-2 py-1">Expires</th>
                <th className="px-2 py-1"></th>
              </tr>
            </thead>
            <tbody>
              {invs.map((i) => (
                <tr key={i.id} className="border-t border-border-subtle">
                  <td className="px-2 py-2">{i.email}</td>
                  <td className="px-2 py-2">{i.role}</td>
                  <td className="px-2 py-2 text-fg-muted">
                    {new Date(i.expires_at).toLocaleString()}
                  </td>
                  <td className="px-2 py-2 text-right">
                    {canManage && (
                      <button onClick={() => cancel(i.id)} className="text-danger hover:underline text-xs">
                        Cancel
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </section>
    </div>
  );
}
