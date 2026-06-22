import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Badge } from "@/components/ui/Badge";
import { CopyButton } from "@/components/ui/CopyButton";
import { FieldLabel } from "@/components/ui/FieldLabel";
import { useConfirm } from "@/hooks/useConfirm";
import { FeatureUnavailableError } from "@/api/client";
import {
  type GitHubTeamGrant,
  type OrgSSOProvider,
  type Role,
  createOrgSSOProvider,
  deleteOrgSSOProvider,
  listOrgSSOProviders,
  testOrgSSOProvider,
  updateOrgSSOProvider,
} from "@/api/orgSso";

// OIDC default-role offers up to admin (owner is never grantable via SSO).
const OIDC_ROLES: Role[] = ["viewer", "member", "admin"];
// GitHub grants are capped at member server-side — only offer viewer/member.
const GITHUB_ROLES: Role[] = ["viewer", "member"];

const EMPTY_OIDC_DRAFT = {
  display_name: "",
  issuer_url: "",
  client_id: "",
  client_secret: "",
  default_role: "member" as Role,
  enabled: true,
};

export default function SSOTab({ teamID, canManage }: { teamID: string; canManage: boolean }) {
  const [providers, setProviders] = useState<OrgSSOProvider[]>([]);
  const [unavailable, setUnavailable] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const reload = async () => {
    setErr(null);
    try {
      setProviders(await listOrgSSOProviders(teamID));
    } catch (e) {
      if (e instanceof FeatureUnavailableError) {
        setUnavailable(true);
        return;
      }
      setErr(errorMessage(e));
    }
  };

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID]);

  const oidc = providers.filter((p) => p.kind === "oidc");
  const github = providers.find((p) => p.kind === "github");

  if (unavailable) {
    return (
      <InlineBanner tone="info" layout="inline">
        Per-org SSO is not enabled on this server.
      </InlineBanner>
    );
  }

  return (
    <div className="space-y-6">
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}
      <KeycloakSection
        teamID={teamID}
        canManage={canManage}
        rows={oidc}
        onChange={reload}
        onError={setErr}
      />
      <GitHubSection
        key={github?.id ?? "new"}
        teamID={teamID}
        canManage={canManage}
        row={github}
        onChange={reload}
        onError={setErr}
      />
    </div>
  );
}

// ---- Keycloak / generic OIDC ----

function KeycloakSection({
  teamID,
  canManage,
  rows,
  onChange,
  onError,
}: {
  teamID: string;
  canManage: boolean;
  rows: OrgSSOProvider[];
  onChange: () => void;
  onError: (m: string) => void;
}) {
  const { confirm, dialog } = useConfirm();
  const [draft, setDraft] = useState(EMPTY_OIDC_DRAFT);
  const [busy, setBusy] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, string>>({});

  const add = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setBusy(true);
    try {
      await createOrgSSOProvider(teamID, { kind: "oidc", ...draft });
      setDraft(EMPTY_OIDC_DRAFT);
      onChange();
    } catch (e) {
      onError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (row: OrgSSOProvider) => {
    try {
      await updateOrgSSOProvider(teamID, row.id, {
        kind: "oidc",
        display_name: row.display_name,
        issuer_url: row.issuer_url,
        client_id: row.client_id,
        scopes: row.scopes,
        default_role: row.default_role,
        enabled: !row.enabled,
      });
      onChange();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  const test = async (row: OrgSSOProvider) => {
    setTestResult((r) => ({ ...r, [row.id]: "…" }));
    try {
      const res = await testOrgSSOProvider(teamID, row.id);
      setTestResult((r) => ({ ...r, [row.id]: res.ok ? "✓ reachable" : `✗ ${res.error ?? "failed"}` }));
    } catch (e) {
      setTestResult((r) => ({ ...r, [row.id]: `✗ ${errorMessage(e)}` }));
    }
  };

  const remove = async (row: OrgSSOProvider) => {
    const ok = await confirm({
      title: "Delete SSO provider?",
      message: `Remove "${row.display_name || row.issuer_url}"? Members who sign in via it will lose that path.`,
      confirmLabel: "Delete",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteOrgSSOProvider(teamID, row.id);
      onChange();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  return (
    <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      {dialog}
      <div>
        <h3 className="font-medium">Keycloak / OIDC</h3>
        <p className="text-sm text-fg-muted">
          Let members sign in via your own OpenID Connect provider. They reach it from the login
          page by entering this org's slug.
        </p>
      </div>

      {rows.length === 0 ? (
        <div className="text-sm text-fg-muted">No OIDC providers configured.</div>
      ) : (
        <ul className="space-y-2">
          {rows.map((row) => (
            <li
              key={row.id}
              className="border border-border-subtle rounded p-3 text-sm space-y-2 bg-surface-0"
            >
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-medium">{row.display_name || "SSO"}</span>
                <Badge variant={row.enabled ? "success" : "neutral"}>
                  {row.enabled ? "enabled" : "disabled"}
                </Badge>
                <span className="text-fg-muted font-mono text-xs break-all">{row.issuer_url}</span>
              </div>
              {row.redirect_uri && (
                <div className="flex items-center gap-2 text-xs text-fg-muted">
                  <span>Redirect URI (register at your IdP):</span>
                  <span className="font-mono break-all">{row.redirect_uri}</span>
                  <CopyButton value={row.redirect_uri} label="Copy" copiedLabel="Copied" />
                </div>
              )}
              {testResult[row.id] && <div className="text-xs">{testResult[row.id]}</div>}
              {canManage && (
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => test(row)}>
                    Test
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => toggle(row)}>
                    {row.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="danger" size="sm" onClick={() => remove(row)}>
                    Delete
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      {canManage && (
        <form onSubmit={add} className="space-y-3 border-t border-border-subtle pt-3">
          <h4 className="text-sm font-medium">Add an OIDC provider</h4>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <FieldLabel htmlFor="sso-display">Display name</FieldLabel>
              <Input
                size="md"
                id="sso-display"
                placeholder="Acme Keycloak"
                value={draft.display_name}
                onChange={(e) => setDraft({ ...draft, display_name: e.target.value })}
              />
            </div>
            <div>
              <FieldLabel htmlFor="sso-issuer">Issuer URL</FieldLabel>
              <Input
                size="md"
                id="sso-issuer"
                type="url"
                placeholder="https://sso.acme.com/realms/main"
                value={draft.issuer_url}
                onChange={(e) => setDraft({ ...draft, issuer_url: e.target.value })}
                required
              />
            </div>
            <div>
              <FieldLabel htmlFor="sso-client">Client ID</FieldLabel>
              <Input
                size="md"
                id="sso-client"
                value={draft.client_id}
                onChange={(e) => setDraft({ ...draft, client_id: e.target.value })}
                required
              />
            </div>
            <div>
              <FieldLabel htmlFor="sso-secret">Client secret</FieldLabel>
              <Input
                size="md"
                id="sso-secret"
                type="password"
                value={draft.client_secret}
                onChange={(e) => setDraft({ ...draft, client_secret: e.target.value })}
                autoComplete="new-password"
                required
              />
            </div>
            <div>
              <FieldLabel htmlFor="sso-role">Default role</FieldLabel>
              <Select
                size="md"
                id="sso-role"
                value={draft.default_role}
                onChange={(e) => setDraft({ ...draft, default_role: e.target.value as Role })}
              >
                {OIDC_ROLES.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </Select>
            </div>
          </div>
          <Checkbox
            label="Enabled"
            checked={draft.enabled}
            onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
          />
          <Button variant="primary" type="submit" loading={busy}>
            Add provider
          </Button>
        </form>
      )}
    </section>
  );
}

// ---- GitHub team gating ----

function GitHubSection({
  teamID,
  canManage,
  row,
  onChange,
  onError,
}: {
  teamID: string;
  canManage: boolean;
  row: OrgSSOProvider | undefined;
  onChange: () => void;
  onError: (m: string) => void;
}) {
  const { confirm, dialog } = useConfirm();
  // Seeded from the stored row. The parent remounts this section
  // (key={row.id}) when a fresh row appears, so prop→state sync needs no
  // effect (and add/remove grant edits stay local until Save).
  const [grants, setGrants] = useState<GitHubTeamGrant[]>(row?.grants ?? []);
  const [enabled, setEnabled] = useState(row?.enabled ?? true);
  const [busy, setBusy] = useState(false);

  const addGrant = () =>
    setGrants((g) => [...g, { github_org: "", team_slug: "", role: "member", verified: false }]);
  const setGrant = (i: number, patch: Partial<GitHubTeamGrant>) =>
    setGrants((g) => g.map((x, j) => (j === i ? { ...x, ...patch } : x)));
  const removeGrant = (i: number) => setGrants((g) => g.filter((_, j) => j !== i));

  const save = async () => {
    setBusy(true);
    try {
      const input = {
        kind: "github" as const,
        enabled,
        // Matched users are always provisioned today; the opt-in "invite-first"
        // (auto_provision=false) gate is a tracked follow-up, so we don't yet
        // surface a control that wouldn't take effect.
        auto_provision: true,
        grants: grants.filter((g) => g.github_org.trim() !== ""),
      };
      if (row) await updateOrgSSOProvider(teamID, row.id, input);
      else await createOrgSSOProvider(teamID, input);
      onChange();
    } catch (e) {
      onError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!row) return;
    const ok = await confirm({
      title: "Remove GitHub team gating?",
      message: "Delete the GitHub allow-list for this org? Members who joined via GitHub keep their access.",
      confirmLabel: "Delete",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteOrgSSOProvider(teamID, row.id);
      onChange();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  return (
    <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      {dialog}
      <div>
        <h3 className="font-medium">GitHub team access</h3>
        <p className="text-sm text-fg-muted">
          Let members of specific GitHub teams sign in with the deployment's "Log in with GitHub"
          button and join this org. Grants are capped at the <strong>member</strong> role.
        </p>
      </div>

      <InlineBanner tone="warning" layout="inline">
        Unverified: iterion does not yet confirm you control these GitHub orgs, so anyone you
        allow-list here can join this org (as a member). Only list GitHub orgs you administer.
      </InlineBanner>

      <div className="space-y-2">
        {grants.length === 0 ? (
          <div className="text-sm text-fg-muted">No grants. Add one to allow a GitHub team in.</div>
        ) : (
          grants.map((g, i) => (
            <div key={i} className="flex gap-2 items-end flex-wrap">
              <div className="flex-1 min-w-32">
                <FieldLabel htmlFor={`gh-org-${i}`}>GitHub org</FieldLabel>
                <Input
                  size="md"
                  id={`gh-org-${i}`}
                  placeholder="acme"
                  value={g.github_org}
                  onChange={(e) => setGrant(i, { github_org: e.target.value })}
                  disabled={!canManage}
                />
              </div>
              <div className="flex-1 min-w-32">
                <FieldLabel htmlFor={`gh-team-${i}`}>Team slug (blank = any)</FieldLabel>
                <Input
                  size="md"
                  id={`gh-team-${i}`}
                  placeholder="engineering"
                  value={g.team_slug ?? ""}
                  onChange={(e) => setGrant(i, { team_slug: e.target.value })}
                  disabled={!canManage}
                />
              </div>
              <div>
                <FieldLabel htmlFor={`gh-role-${i}`}>Role</FieldLabel>
                <Select
                  size="md"
                  id={`gh-role-${i}`}
                  value={g.role}
                  onChange={(e) => setGrant(i, { role: e.target.value as Role })}
                  disabled={!canManage}
                >
                  {GITHUB_ROLES.map((r) => (
                    <option key={r} value={r}>
                      {r}
                    </option>
                  ))}
                </Select>
              </div>
              {canManage && (
                <Button variant="ghost" size="sm" onClick={() => removeGrant(i)}>
                  Remove
                </Button>
              )}
            </div>
          ))
        )}
      </div>

      {canManage && (
        <div className="space-y-3 border-t border-border-subtle pt-3">
          <Checkbox label="Enabled" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={addGrant}>
              Add grant
            </Button>
            <Button variant="primary" size="sm" loading={busy} onClick={save}>
              Save
            </Button>
            {row && (
              <Button variant="danger" size="sm" onClick={remove}>
                Delete allow-list
              </Button>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
