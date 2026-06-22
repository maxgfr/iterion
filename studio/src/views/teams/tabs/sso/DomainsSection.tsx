import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";

import { FeatureUnavailableError } from "@/api/client";
import {
  type OrgDomain,
  addOrgDomain,
  deleteOrgDomain,
  listOrgDomains,
  verifyOrgDomain,
} from "@/api/orgSso";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { CopyButton } from "@/components/ui/CopyButton";
import { FieldLabel } from "@/components/ui/FieldLabel";
import { Input } from "@/components/ui/Input";
import { useAsyncAction } from "@/hooks/useAsyncAction";
import { useConfirm } from "@/hooks/useConfirm";

// Verified email domains — the gate per-org auto-link uses to decide
// whether a fresh OIDC subject can be glued to an existing iterion
// account. Domains start `pending` (DNS TXT record published but not
// yet observed) and flip to `verified` on a successful poll.
export function DomainsSection({
  teamID,
  canManage,
  onError,
}: {
  teamID: string;
  canManage: boolean;
  onError: (m: string) => void;
}) {
  const { confirm, dialog } = useConfirm();
  const [domains, setDomains] = useState<OrgDomain[]>([]);
  const [draft, setDraft] = useState("");
  const [unavailable, setUnavailable] = useState(false);
  const { busy, run } = useAsyncAction();

  const reload = async () => {
    try {
      setDomains(await listOrgDomains(teamID));
    } catch (e) {
      if (e instanceof FeatureUnavailableError) {
        setUnavailable(true);
        return;
      }
      onError(errorMessage(e));
    }
  };
  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID]);

  if (unavailable) {
    return null; // the SSO tab already surfaces the "not enabled" banner
  }

  const add = (ev: React.FormEvent) => {
    ev.preventDefault();
    void run(async () => {
      try {
        await addOrgDomain(teamID, draft);
        setDraft("");
        await reload();
      } catch (e) {
        onError(errorMessage(e));
      }
    });
  };

  const verify = async (d: OrgDomain) => {
    try {
      const r = await verifyOrgDomain(teamID, d.id);
      if (!r.verified) {
        onError(
          `Could not verify ${d.domain} — is the TXT record published? (DNS can take a few minutes.)`,
        );
      }
      await reload();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  const remove = async (d: OrgDomain) => {
    const ok = await confirm({
      title: "Remove domain?",
      message: `Remove ${d.domain}? Auto-link for addresses at this domain will stop.`,
      confirmLabel: "Remove",
      confirmVariant: "danger",
    });
    if (!ok) return;
    await run(async () => {
      try {
        await deleteOrgDomain(teamID, d.id);
        await reload();
      } catch (e) {
        onError(errorMessage(e));
      }
    });
  };

  return (
    <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      {dialog}
      <div>
        <h3 className="font-medium">Verified email domains</h3>
        <p className="text-sm text-fg-muted">
          Prove you control an email domain (via a DNS TXT record) to allow SSO auto-link for its
          addresses. Required for the "auto-link" option above to take effect.
        </p>
      </div>

      {domains.length === 0 ? (
        <div className="text-sm text-fg-muted">No domains.</div>
      ) : (
        <ul className="space-y-2">
          {domains.map((d) => (
            <li
              key={d.id}
              className="border border-border-subtle rounded p-3 text-sm space-y-2 bg-surface-0"
            >
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-medium">{d.domain}</span>
                <Badge variant={d.verified_at ? "success" : "neutral"}>
                  {d.verified_at ? "verified" : "pending"}
                </Badge>
              </div>
              {!d.verified_at && (
                <div className="text-xs text-fg-muted space-y-1">
                  <div>Publish this DNS TXT record, then click Verify:</div>
                  <div className="flex items-center gap-2">
                    <span className="font-mono break-all">{d.challenge_host}</span>
                    <CopyButton value={d.challenge_host} label="Copy" copiedLabel="Copied" />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="font-mono break-all">{d.challenge_value}</span>
                    <CopyButton value={d.challenge_value} label="Copy" copiedLabel="Copied" />
                  </div>
                </div>
              )}
              {canManage && (
                <div className="flex gap-2">
                  {!d.verified_at && (
                    <Button variant="secondary" size="sm" onClick={() => void verify(d)}>
                      Verify
                    </Button>
                  )}
                  <Button variant="danger" size="sm" onClick={() => void remove(d)}>
                    Remove
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      {canManage && (
        <form onSubmit={add} className="flex gap-2 items-end border-t border-border-subtle pt-3">
          <div className="flex-1">
            <FieldLabel htmlFor="sso-domain">Domain</FieldLabel>
            <Input
              size="md"
              id="sso-domain"
              placeholder="acme.com"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              required
            />
          </div>
          <Button variant="primary" type="submit" loading={busy}>
            Add domain
          </Button>
        </form>
      )}
    </section>
  );
}
