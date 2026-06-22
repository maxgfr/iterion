import { errorMessage } from "@/lib/errorHints";
import { useState } from "react";

import {
  type OrgSSOProvider,
  type Role,
  createOrgSSOProvider,
  deleteOrgSSOProvider,
  testOrgSSOProvider,
  updateOrgSSOProvider,
} from "@/api/orgSso";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { CopyButton } from "@/components/ui/CopyButton";
import { FieldLabel } from "@/components/ui/FieldLabel";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useAsyncAction } from "@/hooks/useAsyncAction";
import { useConfirm } from "@/hooks/useConfirm";

// OIDC default-role offers up to admin (owner is never grantable via SSO).
const OIDC_ROLES: Role[] = ["viewer", "member", "admin"];

const EMPTY_OIDC_DRAFT = {
  display_name: "",
  issuer_url: "",
  client_id: "",
  client_secret: "",
  default_role: "member" as Role,
  enabled: true,
  auto_link_on_email: false,
};

// Keycloak/OIDC providers section. Lists existing providers (with a
// Test button that pings the IdP), and an add form below. Toggling
// enabled and deleting go through useAsyncAction so the parent's
// global err banner shows mutation failures with a single channel.
export function KeycloakSection({
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
  const [testResult, setTestResult] = useState<Record<string, string>>({});
  const { busy, run } = useAsyncAction();

  const add = (ev: React.FormEvent) => {
    ev.preventDefault();
    void run(async () => {
      try {
        await createOrgSSOProvider(teamID, { kind: "oidc", ...draft });
        setDraft(EMPTY_OIDC_DRAFT);
        onChange();
      } catch (e) {
        onError(errorMessage(e));
      }
    });
  };

  const toggle = (row: OrgSSOProvider) =>
    run(async () => {
      try {
        await updateOrgSSOProvider(teamID, row.id, {
          kind: "oidc",
          display_name: row.display_name,
          issuer_url: row.issuer_url,
          client_id: row.client_id,
          scopes: row.scopes,
          default_role: row.default_role,
          auto_link_on_email: row.auto_link_on_email,
          enabled: !row.enabled,
        });
        onChange();
      } catch (e) {
        onError(errorMessage(e));
      }
    });

  const test = async (row: OrgSSOProvider) => {
    setTestResult((r) => ({ ...r, [row.id]: "…" }));
    try {
      const res = await testOrgSSOProvider(teamID, row.id);
      setTestResult((r) => ({
        ...r,
        [row.id]: res.ok ? "✓ reachable" : `✗ ${res.error ?? "failed"}`,
      }));
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
    await run(async () => {
      try {
        await deleteOrgSSOProvider(teamID, row.id);
        onChange();
      } catch (e) {
        onError(errorMessage(e));
      }
    });
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
                {row.auto_link_on_email && <Badge variant="neutral">auto-link</Badge>}
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
                  <Button variant="secondary" size="sm" onClick={() => void test(row)}>
                    Test
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => void toggle(row)}>
                    {row.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="danger" size="sm" onClick={() => void remove(row)}>
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
          <Checkbox
            label="Auto-link existing accounts whose email is at a verified domain"
            help="When on, a user who already has an iterion account is linked automatically on first SSO login — but only for email domains you've verified below. Otherwise they must link from settings."
            checked={draft.auto_link_on_email}
            onChange={(e) => setDraft({ ...draft, auto_link_on_email: e.target.checked })}
          />
          <Button variant="primary" type="submit" loading={busy}>
            Add provider
          </Button>
        </form>
      )}
    </section>
  );
}
