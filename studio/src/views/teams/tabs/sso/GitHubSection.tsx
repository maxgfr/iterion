import { errorMessage } from "@/lib/errorHints";
import { useState } from "react";

import {
  type GitHubTeamGrant,
  type OrgSSOProvider,
  type Role,
  createOrgSSOProvider,
  deleteOrgSSOProvider,
  updateOrgSSOProvider,
} from "@/api/orgSso";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { FieldLabel } from "@/components/ui/FieldLabel";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useAsyncAction } from "@/hooks/useAsyncAction";
import { useConfirm } from "@/hooks/useConfirm";

// GitHub grants are capped at member server-side — only offer viewer/member.
const GITHUB_ROLES: Role[] = ["viewer", "member"];

// GitHub team-gating section. Up to one provider per org; grants are
// edit-locally / save-explicit so the operator can stage multiple
// teams without committing each.
export function GitHubSection({
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
  const { busy, run } = useAsyncAction();

  const addGrant = () =>
    setGrants((g) => [...g, { github_org: "", team_slug: "", role: "member", verified: false }]);
  const setGrant = (i: number, patch: Partial<GitHubTeamGrant>) =>
    setGrants((g) => g.map((x, j) => (j === i ? { ...x, ...patch } : x)));
  const removeGrant = (i: number) => setGrants((g) => g.filter((_, j) => j !== i));

  const save = () =>
    run(async () => {
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
      }
    });

  const remove = async () => {
    if (!row) return;
    const ok = await confirm({
      title: "Remove GitHub team gating?",
      message:
        "Delete the GitHub allow-list for this org? Members who joined via GitHub keep their access.",
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
        <h3 className="font-medium">GitHub team access</h3>
        <p className="text-sm text-fg-muted">
          Let members of specific GitHub teams sign in with the deployment's "Log in with GitHub"
          button and join this org. Grants are capped at the <strong>member</strong> role.
        </p>
      </div>

      <InlineBanner tone="warning" layout="inline">
        A grant takes effect only once iterion verifies you administer its GitHub org — connect
        that org under the Integrations tab first. Unverified grants are saved but inert.
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
              <Badge variant={g.verified ? "success" : "neutral"}>
                {g.verified ? "verified" : "pending"}
              </Badge>
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
            <Button variant="primary" size="sm" loading={busy} onClick={() => void save()}>
              Save
            </Button>
            {row && (
              <Button variant="danger" size="sm" onClick={() => void remove()}>
                Delete allow-list
              </Button>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
