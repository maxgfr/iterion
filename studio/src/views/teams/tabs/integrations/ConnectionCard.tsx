import { errorMessage } from "@/lib/errorHints";
import { useState } from "react";

import type { BotEntryWithSchema } from "@/api/bots";
import {
  type ForgeConnection,
  type ForgeIntegration,
  deleteForgeConnection,
  disableForgeIntegration,
} from "@/api/forgeConnections";
import { InlineBanner } from "@/components/ui/InlineBanner";

import { type ConfirmFn, statusTone } from "./forgeShared";
import { EnableRepoPanel } from "./EnableRepoPanel";

export function ConnectionCard({
  teamID,
  conn,
  integrations,
  forgeBots,
  canManage,
  onChanged,
  onError,
  confirm,
  preselectBot,
  autoOpenEnable,
}: {
  teamID: string;
  conn: ForgeConnection;
  integrations: ForgeIntegration[];
  forgeBots: BotEntryWithSchema[];
  canManage: boolean;
  onChanged: () => void;
  onError: (m: string) => void;
  confirm: ConfirmFn;
  preselectBot?: string;
  autoOpenEnable?: boolean;
}) {
  const [enabling, setEnabling] = useState(!!autoOpenEnable);

  const disconnect = async () => {
    const ok = await confirm({
      title: "Disconnect forge?",
      message: `Disconnecting removes every webhook iterion created on ${conn.account_login ?? conn.provider} (${integrations.length} repo${integrations.length === 1 ? "" : "s"}).`,
      confirmLabel: "Disconnect",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteForgeConnection(teamID, conn.id);
      onChanged();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  const disable = async (i: ForgeIntegration) => {
    const ok = await confirm({
      title: "Disable on this repo?",
      message: `Remove the iterion webhook from ${i.repo_full_name}?`,
      confirmLabel: "Disable",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await disableForgeIntegration(teamID, i.id);
      onChanged();
    } catch (e) {
      onError(errorMessage(e));
    }
  };

  return (
    <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="font-medium">
            {conn.provider} · @{conn.account_login ?? "—"}
            <InlineBanner tone={statusTone(conn.status)} layout="inline" className="ml-2 inline-flex">
              {conn.status}
            </InlineBanner>
          </div>
          <div className="text-xs text-fg-muted">
            {conn.forge_base_url ?? conn.provider} · {conn.kind}
          </div>
        </div>
        {canManage && (
          <button
            type="button"
            onClick={disconnect}
            className="text-danger hover:underline text-xs"
          >
            Disconnect
          </button>
        )}
      </div>

      <div>
        <div className="text-xs uppercase tracking-wider text-fg-muted mb-1">Enabled repos</div>
        {integrations.length === 0 ? (
          <div className="text-fg-muted text-sm">None yet.</div>
        ) : (
          <ul className="space-y-1">
            {integrations.map((i) => (
              <li
                key={i.id}
                className="flex items-center justify-between gap-2 text-sm border-t border-border-subtle pt-1"
              >
                <span>
                  <span className="font-mono">{i.repo_full_name}</span>{" "}
                  <span className="text-fg-muted">· {i.bot_ids.join(", ")}</span>
                </span>
                {canManage && (
                  <button
                    type="button"
                    onClick={() => disable(i)}
                    className="text-danger hover:underline text-xs"
                  >
                    Disable
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {canManage &&
        (enabling ? (
          <EnableRepoPanel
            teamID={teamID}
            conn={conn}
            forgeBots={forgeBots}
            preselectBot={preselectBot}
            onDone={() => {
              setEnabling(false);
              onChanged();
            }}
            onCancel={() => setEnabling(false)}
            onError={onError}
          />
        ) : (
          <button
            type="button"
            onClick={() => setEnabling(true)}
            className="text-accent-text hover:underline text-sm"
          >
            + Enable a repo
          </button>
        ))}
    </section>
  );
}
