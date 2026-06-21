import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";
import { useSearch } from "wouter";

import { type BotEntryWithSchema, listBots } from "@/api/bots";
import { FeatureUnavailableError } from "@/api/client";
import {
  type ForgeConnection,
  type ForgeIntegration,
  type ForgeOAuthApp,
  listForgeConnections,
  listForgeIntegrations,
  listForgeOAuthApps,
} from "@/api/forgeConnections";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useConfirm } from "@/hooks/useConfirm";

import { ConnectForm } from "./integrations/ConnectForm";
import { ConnectionCard } from "./integrations/ConnectionCard";
import { OAuthAppsSection } from "./integrations/OAuthAppsSection";

export default function IntegrationsTab({
  teamID,
  canManage,
}: {
  teamID: string;
  canManage: boolean;
}) {
  const [connections, setConnections] = useState<ForgeConnection[]>([]);
  const [integrations, setIntegrations] = useState<ForgeIntegration[]>([]);
  const [oauthApps, setOAuthApps] = useState<ForgeOAuthApp[]>([]);
  const [forgeBots, setForgeBots] = useState<BotEntryWithSchema[]>([]);
  const [unavailable, setUnavailable] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [botsWarning, setBotsWarning] = useState<string | null>(null);
  const { confirm, dialog } = useConfirm();
  // ?bot=<name> (set by the catalog's "Connect to a repo" affordance) pre-checks
  // that bot in the enable dialog and auto-opens it when there's one connection.
  const preselectBot = new URLSearchParams(useSearch()).get("bot") ?? undefined;

  const reload = async () => {
    setErr(null);
    try {
      const [conns, ints, apps] = await Promise.all([
        listForgeConnections(teamID),
        listForgeIntegrations(teamID),
        listForgeOAuthApps(teamID),
      ]);
      setConnections(conns);
      setIntegrations(ints);
      setOAuthApps(apps);
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
    void listBots()
      .then((bots) => {
        setForgeBots(bots.filter((b) => b.forge));
        setBotsWarning(null);
      })
      .catch((e) =>
        setBotsWarning(
          (e as Error)?.message ?? "Failed to load forge-capable bots.",
        ),
      );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [teamID]);

  if (unavailable) {
    return (
      <InlineBanner tone="info" layout="inline">
        Forge integrations are not enabled on this server. They require the cloud control
        plane (Mongo-backed connection + webhook stores).
      </InlineBanner>
    );
  }

  return (
    <div className="space-y-6">
      {dialog}
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}
      {botsWarning && (
        <InlineBanner tone="warning" layout="inline">
          {botsWarning}
        </InlineBanner>
      )}

      <div>
        <h3 className="font-medium mb-1">Connected forges</h3>
        <p className="text-xs text-fg-muted mb-3">
          Connect a GitLab/GitHub/Forgejo account once, then enable a bot on a repo — iterion
          creates the webhook on the forge and wires the bot's token for you.
        </p>
        {connections.length === 0 ? (
          <div className="text-fg-muted text-sm">No forge connected yet.</div>
        ) : (
          <div className="space-y-3">
            {connections.map((c) => (
              <ConnectionCard
                key={c.id}
                teamID={teamID}
                conn={c}
                integrations={integrations.filter((i) => i.connection_id === c.id)}
                forgeBots={forgeBots}
                canManage={canManage}
                onChanged={reload}
                onError={setErr}
                confirm={confirm}
                preselectBot={preselectBot}
                autoOpenEnable={!!preselectBot && connections.length === 1}
              />
            ))}
          </div>
        )}
      </div>

      <OAuthAppsSection
        teamID={teamID}
        apps={oauthApps}
        connections={connections}
        canManage={canManage}
        onChanged={reload}
        onError={setErr}
        confirm={confirm}
      />

      {canManage && (
        <ConnectForm
          teamID={teamID}
          oauthApps={oauthApps}
          onConnected={reload}
          onError={setErr}
        />
      )}
    </div>
  );
}
