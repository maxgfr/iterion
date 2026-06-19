import { useEffect, useMemo, useState } from "react";
import { useSearch } from "wouter";

import { type BotEntryWithSchema, listBots } from "@/api/bots";
import { FeatureUnavailableError } from "@/api/client";
import {
  type ForgeConnection,
  type ForgeEnablePreview,
  type ForgeIntegration,
  type ForgeProvider,
  type ForgeRepo,
  connectForge,
  deleteForgeConnection,
  disableForgeIntegration,
  enableForgeRepoBots,
  listForgeConnections,
  listForgeIntegrations,
  listForgeRepos,
  previewForgeEnable,
} from "@/api/forgeConnections";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useConfirm } from "@/hooks/useConfirm";

// All three forges have wired admin clients (PAT + OAuth App). GitHub App
// (installation-token) is a separate connect mode handled server-side.
const CONNECTABLE: ForgeProvider[] = ["gitlab", "github", "forgejo"];

export default function IntegrationsTab({
  teamID,
  canManage,
}: {
  teamID: string;
  canManage: boolean;
}) {
  const [connections, setConnections] = useState<ForgeConnection[]>([]);
  const [integrations, setIntegrations] = useState<ForgeIntegration[]>([]);
  const [forgeBots, setForgeBots] = useState<BotEntryWithSchema[]>([]);
  const [unavailable, setUnavailable] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const { confirm, dialog } = useConfirm();
  // ?bot=<name> (set by the catalog's "Connect to a repo" affordance) pre-checks
  // that bot in the enable dialog and auto-opens it when there's one connection.
  const preselectBot = new URLSearchParams(useSearch()).get("bot") ?? undefined;

  const reload = async () => {
    setErr(null);
    try {
      const [conns, ints] = await Promise.all([
        listForgeConnections(teamID),
        listForgeIntegrations(teamID),
      ]);
      setConnections(conns);
      setIntegrations(ints);
    } catch (e) {
      if (e instanceof FeatureUnavailableError) {
        setUnavailable(true);
        return;
      }
      setErr((e as Error).message);
    }
  };

  useEffect(() => {
    void reload();
    void listBots()
      .then((bots) => setForgeBots(bots.filter((b) => b.forge)))
      .catch(() => {});
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

      {canManage && <ConnectForm teamID={teamID} onConnected={reload} onError={setErr} />}
    </div>
  );
}

function statusTone(status: ForgeConnection["status"]): "success" | "warning" | "danger" {
  if (status === "active") return "success";
  if (status === "needs_reauth") return "warning";
  return "danger";
}

function ConnectionCard({
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
  confirm: ReturnType<typeof useConfirm>["confirm"];
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
      onError((e as Error).message);
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
      onError((e as Error).message);
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
          <button onClick={disconnect} className="text-danger hover:underline text-xs">
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
                  <button onClick={() => disable(i)} className="text-danger hover:underline text-xs">
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
            onClick={() => setEnabling(true)}
            className="text-fg-accent hover:underline text-sm"
          >
            + Enable a repo
          </button>
        ))}
    </section>
  );
}

function EnableRepoPanel({
  teamID,
  conn,
  forgeBots,
  preselectBot,
  onDone,
  onCancel,
  onError,
}: {
  teamID: string;
  conn: ForgeConnection;
  forgeBots: BotEntryWithSchema[];
  preselectBot?: string;
  onDone: () => void;
  onCancel: () => void;
  onError: (m: string) => void;
}) {
  const [search, setSearch] = useState("");
  const [repos, setRepos] = useState<ForgeRepo[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [repo, setRepo] = useState("");
  const [selectedBots, setSelectedBots] = useState<string[]>(
    preselectBot ? [preselectBot] : [],
  );
  const [preview, setPreview] = useState<ForgeEnablePreview | null>(null);
  const [busy, setBusy] = useState(false);

  const loadRepos = async () => {
    setLoadingRepos(true);
    try {
      setRepos(await listForgeRepos(teamID, conn.id, search));
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setLoadingRepos(false);
    }
  };

  useEffect(() => {
    void loadRepos();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fetch the authoritative preview (native events the hook will subscribe
  // to + identity + any scope/forge-block conflicts) whenever the selection
  // changes, so the operator sees exactly what Enable will provision.
  useEffect(() => {
    if (!repo || selectedBots.length === 0) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    void previewForgeEnable(teamID, conn.id, repo, selectedBots)
      .then((p) => {
        if (!cancelled) setPreview(p);
      })
      .catch(() => {
        if (!cancelled) setPreview(null);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo, selectedBots]);

  const toggleBot = (name: string) =>
    setSelectedBots((s) => (s.includes(name) ? s.filter((b) => b !== name) : [...s, name]));

  const hasConflicts = (preview?.conflicts?.length ?? 0) > 0;

  const enable = async () => {
    if (!repo || selectedBots.length === 0) return;
    setBusy(true);
    try {
      await enableForgeRepoBots(teamID, conn.id, repo, selectedBots);
      onDone();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="bg-surface-0 border border-border-subtle rounded p-3 space-y-3">
      <div className="flex gap-2">
        <input
          className="flex-1 bg-surface-1 border border-border-subtle rounded px-2 py-1 text-sm"
          placeholder="Search repos…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void loadRepos();
          }}
        />
        <button
          onClick={() => void loadRepos()}
          className="text-sm border border-border-subtle rounded px-2 py-1 hover:bg-surface-2"
        >
          {loadingRepos ? "…" : "Search"}
        </button>
      </div>

      <select
        className="w-full bg-surface-1 border border-border-subtle rounded px-2 py-1 text-sm"
        value={repo}
        onChange={(e) => setRepo(e.target.value)}
      >
        <option value="">Select a repository…</option>
        {repos.map((r) => (
          <option key={r.full_name} value={r.full_name} disabled={!r.can_admin}>
            {r.full_name}
            {r.can_admin ? "" : " (no admin access)"}
          </option>
        ))}
      </select>

      <div>
        <div className="text-xs uppercase tracking-wider text-fg-muted mb-1">Bots to enable</div>
        {forgeBots.length === 0 ? (
          <div className="text-fg-muted text-sm">
            No forge-capable bots found (a bot needs a <span className="font-mono">forge:</span>{" "}
            block in its manifest).
          </div>
        ) : (
          <ul className="space-y-2">
            {forgeBots.map((b) => (
              <li key={b.name} className="flex gap-2">
                <input
                  type="checkbox"
                  id={`fb-${b.name}`}
                  checked={selectedBots.includes(b.name)}
                  onChange={() => toggleBot(b.name)}
                  className="mt-1"
                />
                <label htmlFor={`fb-${b.name}`} className="text-sm">
                  <span className="font-medium">{b.display_name || b.name}</span>
                  {b.forge?.events && b.forge.events.length > 0 && (
                    <span className="text-fg-muted">
                      {" "}
                      · subscribes to {b.forge.events.join(", ")}
                    </span>
                  )}
                  {b.forge?.rationale && (
                    <span className="block text-caption text-fg-muted whitespace-pre-wrap">
                      {b.forge.rationale}
                    </span>
                  )}
                </label>
              </li>
            ))}
          </ul>
        )}
      </div>

      {preview && (
        <div className="bg-surface-1 border border-border-subtle rounded p-2 text-xs space-y-1">
          {preview.forge_native_events.length > 0 && (
            <div>
              <span className="text-fg-muted">Will subscribe to:</span>{" "}
              <span className="font-mono">{preview.forge_native_events.join(", ")}</span>
            </div>
          )}
          {preview.identity.handle && (
            <div>
              <span className="text-fg-muted">Will post as:</span> @{preview.identity.handle}
            </div>
          )}
          {hasConflicts &&
            preview.conflicts.map((c) => (
              <div key={c} className="text-danger">
                ⚠ {c}
              </div>
            ))}
        </div>
      )}

      <div className="flex items-center gap-2">
        <button
          onClick={() => void enable()}
          disabled={busy || !repo || selectedBots.length === 0 || hasConflicts}
          className="bg-accent text-fg-onAccent rounded px-3 py-1 text-sm disabled:opacity-50"
        >
          {busy ? "Enabling…" : "Enable"}
        </button>
        <button onClick={onCancel} className="text-fg-muted hover:text-fg-default text-sm">
          Cancel
        </button>
      </div>
    </div>
  );
}

function ConnectForm({
  teamID,
  onConnected,
  onError,
}: {
  teamID: string;
  onConnected: () => void;
  onError: (m: string) => void;
}) {
  const [provider, setProvider] = useState<ForgeProvider>("gitlab");
  const [baseURL, setBaseURL] = useState("");
  const [mode, setMode] = useState<"oauth" | "pat" | "app">("pat");
  const [pat, setPat] = useState("");
  const [busy, setBusy] = useState(false);

  const pickProvider = (p: ForgeProvider) => {
    setProvider(p);
    if (p !== "github" && mode === "app") setMode("pat");
  };

  const connect = async () => {
    setBusy(true);
    try {
      const res = await connectForge(teamID, {
        provider,
        mode,
        forge_base_url: baseURL.trim() || undefined,
        pat: mode === "pat" ? pat : undefined,
        next: window.location.pathname,
      });
      // full-page round-trip for OAuth / App install.
      if (res.authorize_url || res.install_url) {
        window.location.href = (res.authorize_url ?? res.install_url) as string;
        return;
      }
      setPat("");
      onConnected();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const redirectHint = useMemo(() => {
    if (mode === "oauth") return "You'll be redirected to authorize iterion, then back here.";
    if (mode === "app") return "You'll be redirected to GitHub to install the app, then back here.";
    return "";
  }, [mode]);

  return (
    <section className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      <h3 className="font-medium">Connect a forge</h3>
      <div className="flex gap-2 flex-wrap">
        {(["gitlab", "github", "forgejo"] as ForgeProvider[]).map((p) => (
          <button
            key={p}
            disabled={!CONNECTABLE.includes(p)}
            onClick={() => pickProvider(p)}
            className={`text-sm rounded px-3 py-1 border ${
              provider === p ? "border-accent bg-surface-2" : "border-border-subtle"
            } disabled:opacity-40`}
            title={CONNECTABLE.includes(p) ? "" : "Coming in a later phase"}
          >
            {p}
            {CONNECTABLE.includes(p) ? "" : " (soon)"}
          </button>
        ))}
      </div>

      <input
        className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2 text-sm"
        placeholder="Forge base URL (optional — for self-hosted, e.g. https://gitlab.example.com)"
        value={baseURL}
        onChange={(e) => setBaseURL(e.target.value)}
      />

      <div className="flex gap-3 text-sm">
        <label className="flex items-center gap-1">
          <input
            type="radio"
            checked={mode === "pat"}
            onChange={() => setMode("pat")}
          />
          Paste a token
        </label>
        <label className="flex items-center gap-1">
          <input
            type="radio"
            checked={mode === "oauth"}
            onChange={() => setMode("oauth")}
          />
          Use OAuth
        </label>
        {provider === "github" && (
          <label className="flex items-center gap-1">
            <input type="radio" checked={mode === "app"} onChange={() => setMode("app")} />
            Install GitHub App
          </label>
        )}
      </div>

      {mode === "pat" && (
        <input
          type="password"
          className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2 text-sm"
          placeholder="Personal access token (api / repo + hook-admin scope)"
          value={pat}
          onChange={(e) => setPat(e.target.value)}
        />
      )}
      {redirectHint && <p className="text-caption text-fg-muted">{redirectHint}</p>}

      <button
        onClick={() => void connect()}
        disabled={busy || (mode === "pat" && pat.trim() === "")}
        className="bg-accent text-fg-onAccent rounded px-3 py-2 text-sm disabled:opacity-50"
      >
        {busy ? "Connecting…" : "Connect"}
      </button>
    </section>
  );
}
