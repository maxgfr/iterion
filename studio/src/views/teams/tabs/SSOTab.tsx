import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";

import { FeatureUnavailableError } from "@/api/client";
import { type OrgSSOProvider, listOrgSSOProviders } from "@/api/orgSso";
import { InlineBanner } from "@/components/ui/InlineBanner";

import { DomainsSection } from "./sso/DomainsSection";
import { GitHubSection } from "./sso/GitHubSection";
import { KeycloakSection } from "./sso/KeycloakSection";

// SSOTab orchestrates the three per-org SSO sections: Keycloak/OIDC
// providers, verified email domains (the auto-link gate), and the
// GitHub team-gating allow-list. The actual section logic lives in
// ./sso/ — this file only fetches the provider list and routes it.
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
      <DomainsSection teamID={teamID} canManage={canManage} onError={setErr} />
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
