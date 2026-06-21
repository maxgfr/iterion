import { errorMessage } from "@/lib/errorHints";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearch } from "wouter";

import { type BotEntryWithSchema, listBots } from "@/api/bots";
import { FeatureUnavailableError } from "@/api/client";
import {
  type ForgeConnection,
  type ForgeEnablePreview,
  type ForgeIntegration,
  type ForgeOAuthApp,
  type ForgeProvider,
  type ForgeRepo,
  type RegisterForgeOAuthAppInput,
  connectForge,
  deleteForgeConnection,
  deleteForgeOAuthApp,
  disableForgeIntegration,
  enableForgeRepoBots,
  listForgeConnections,
  listForgeIntegrations,
  listForgeOAuthApps,
  listForgeRepos,
  previewForgeEnable,
  registerForgeOAuthApp,
  startGitHubManifest,
} from "@/api/forgeConnections";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useConfirm } from "@/hooks/useConfirm";

// canonicalBase mirrors forge.CanonicalBaseURL (Go) so the connect form can
// match a typed base URL against a stored OAuth app's instance key.
const DEFAULT_BASE: Record<ForgeProvider, string> = {
  gitlab: "https://gitlab.com",
  github: "https://github.com",
  forgejo: "https://codeberg.org",
};
function canonicalBase(provider: ForgeProvider, raw: string): string {
  const s = raw.trim();
  if (!s) return DEFAULT_BASE[provider];
  const withScheme = s.includes("://") ? s : `https://${s}`;
  return withScheme.replace(/\/+$/, "");
}

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
      onError(errorMessage(e));
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
      onError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="bg-surface-0 border border-border-subtle rounded p-3 space-y-3">
      <div className="flex gap-2 items-center">
        <div className="flex-1">
          <label htmlFor="forge-repo-search" className="sr-only">
            Search repos
          </label>
          <Input
            id="forge-repo-search"
            placeholder="Search repos…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void loadRepos();
            }}
          />
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => void loadRepos()}
          loading={loadingRepos}
        >
          {loadingRepos ? "…" : "Search"}
        </Button>
      </div>

      <div>
        <label htmlFor="forge-repo-pick" className="sr-only">
          Repository
        </label>
        <Select
          id="forge-repo-pick"
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
        </Select>
      </div>

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
        <Button
          variant="primary"
          onClick={() => void enable()}
          disabled={busy || !repo || selectedBots.length === 0 || hasConflicts}
          loading={busy}
        >
          {busy ? "Enabling…" : "Enable"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

function ConnectForm({
  teamID,
  oauthApps,
  onConnected,
  onError,
}: {
  teamID: string;
  oauthApps: ForgeOAuthApp[];
  onConnected: () => void;
  onError: (m: string) => void;
}) {
  const [provider, setProvider] = useState<ForgeProvider>("gitlab");
  const [baseURL, setBaseURL] = useState("");
  const [mode, setMode] = useState<"oauth" | "pat" | "app">("oauth");
  const [pat, setPat] = useState("");
  const [busy, setBusy] = useState(false);
  // Once the user picks a mode explicitly, stop auto-steering it.
  const modeTouched = useRef(false);

  // OAuth is offered for a (provider, instance) only when a matching OAuth app
  // is registered for this team; otherwise the PAT fallback.
  const appExists = (p: ForgeProvider, base: string) =>
    oauthApps.some(
      (a) => a.provider === p && (a.forge_base_url ?? DEFAULT_BASE[p]) === canonicalBase(p, base),
    );
  const oauthAvailable = appExists(provider, baseURL);

  // Steer to OAuth when an app exists for the selected (provider, instance),
  // else PAT — re-runs when the match flips, unless the user overrode it.
  useEffect(() => {
    if (modeTouched.current) return;
    setMode(oauthAvailable ? "oauth" : "pat");
  }, [oauthAvailable]);

  const pickMode = (m: "oauth" | "pat" | "app") => {
    modeTouched.current = true;
    setMode(m);
  };

  const pickProvider = (p: ForgeProvider) => {
    setProvider(p);
    // Re-steer to the new provider's best default (also clears a stale,
    // github-only "app" mode when switching away).
    modeTouched.current = false;
    setMode(appExists(p, baseURL) ? "oauth" : "pat");
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
      const msg = errorMessage(e);
      // Self-hosted forges (e.g. a private GitLab) usually have no OAuth app
      // registered on this server. Rather than dead-ending on the raw 400,
      // steer the user to the PAT path (which is always available).
      if (mode === "oauth" && /no oauth app is registered|oauth is not configured/i.test(msg)) {
        modeTouched.current = true;
        setMode("pat");
        onError(
          "No OAuth app is registered for this instance — register one above, or paste a personal access token instead (now selected below).",
        );
      } else {
        onError(msg);
      }
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
            type="button"
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

      <div>
        <label htmlFor="forge-base-url" className="sr-only">
          Forge base URL
        </label>
        <Input
          size="md"
          id="forge-base-url"
          placeholder="Forge base URL (optional — for self-hosted, e.g. https://gitlab.example.com)"
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
        />
      </div>

      <div className="flex gap-3 text-sm">
        <label
          className={`flex items-center gap-1 ${oauthAvailable ? "" : "opacity-50"}`}
          title={
            oauthAvailable
              ? ""
              : `No OAuth app registered for ${provider} on this instance — register one above, or paste a token`
          }
        >
          <input
            type="radio"
            checked={mode === "oauth"}
            onChange={() => pickMode("oauth")}
            disabled={!oauthAvailable}
          />
          Use OAuth{oauthAvailable ? "" : " (no app)"}
        </label>
        <label className="flex items-center gap-1">
          <input
            type="radio"
            checked={mode === "pat"}
            onChange={() => pickMode("pat")}
          />
          Paste a token
        </label>
        {provider === "github" && (
          <label className="flex items-center gap-1">
            <input type="radio" checked={mode === "app"} onChange={() => pickMode("app")} />
            Install GitHub App
          </label>
        )}
      </div>

      {mode === "pat" && (
        <div>
          <label htmlFor="forge-pat" className="sr-only">
            Personal access token
          </label>
          <Input
            size="md"
            type="password"
            id="forge-pat"
            placeholder="Personal access token (api / repo + hook-admin scope)"
            value={pat}
            onChange={(e) => setPat(e.target.value)}
            autoComplete="off"
          />
        </div>
      )}
      {redirectHint && <p className="text-caption text-fg-muted">{redirectHint}</p>}

      <Button
        variant="primary"
        onClick={() => void connect()}
        disabled={busy || (mode === "pat" && pat.trim() === "")}
        loading={busy}
      >
        {busy ? "Connecting…" : "Connect"}
      </Button>
    </section>
  );
}

function OAuthAppsSection({
  teamID,
  apps,
  connections,
  canManage,
  onChanged,
  onError,
  confirm,
}: {
  teamID: string;
  apps: ForgeOAuthApp[];
  connections: ForgeConnection[];
  canManage: boolean;
  onChanged: () => void;
  onError: (m: string) => void;
  confirm: ReturnType<typeof useConfirm>["confirm"];
}) {
  const remove = async (a: ForgeOAuthApp) => {
    const ok = await confirm({
      title: "Delete OAuth app?",
      message: `Connections that authenticate via this ${a.provider} app (${a.forge_base_url ?? a.provider}) will no longer be able to OAuth-refresh. Existing connections keep working until their token expires.`,
      confirmLabel: "Delete",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteForgeOAuthApp(teamID, a.id);
      onChanged();
    } catch (e) {
      onError((e as Error).message);
    }
  };

  return (
    <div>
      <h3 className="font-medium mb-1">Forge OAuth apps</h3>
      <p className="text-xs text-fg-muted mb-3">
        Register an OAuth application per forge instance to connect over OAuth instead of a personal
        access token. Scoped to this team — each forge and self-hosted instance can have its own app.
      </p>
      {apps.length === 0 ? (
        <div className="text-fg-muted text-sm">No OAuth app registered yet.</div>
      ) : (
        <ul className="space-y-2">
          {apps.map((a) => (
            <li
              key={a.id}
              className="flex items-center justify-between gap-2 bg-surface-1 border border-border-subtle rounded px-3 py-2 text-sm"
            >
              <div className="min-w-0">
                <div className="font-medium">
                  {a.provider} · {a.forge_base_url ?? "—"}
                  <span className="ml-2 rounded bg-surface-2 px-1 text-caption text-fg-subtle">
                    {a.auto_created ? "auto" : "manual"}
                  </span>
                </div>
                <div className="text-caption text-fg-muted font-mono truncate">
                  client_id: {a.client_id}
                </div>
              </div>
              {canManage && (
                <button
                  type="button"
                  onClick={() => void remove(a)}
                  className="text-danger hover:underline text-xs shrink-0"
                >
                  Delete
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canManage && (
        <RegisterOAuthAppForm
          teamID={teamID}
          connections={connections}
          onRegistered={onChanged}
          onError={onError}
        />
      )}
    </div>
  );
}

type RegisterMode = "auto" | "auto_from_connection" | "manual";

function RegisterOAuthAppForm({
  teamID,
  connections,
  onRegistered,
  onError,
}: {
  teamID: string;
  connections: ForgeConnection[];
  onRegistered: () => void;
  onError: (m: string) => void;
}) {
  const [show, setShow] = useState(false);
  const [provider, setProvider] = useState<ForgeProvider>("gitlab");
  const [baseURL, setBaseURL] = useState("");
  const [mode, setMode] = useState<RegisterMode>("auto");
  const [adminToken, setAdminToken] = useState("");
  const [connectionID, setConnectionID] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [busy, setBusy] = useState(false);

  const redirectURI = `${window.location.origin}/api/forge/oauth/callback`;
  // GitHub has no create-app REST API (only the interactive App-Manifest flow),
  // so token-based auto-create isn't available for it yet — nudge to manual.
  const autoSupported = provider !== "github";
  const usableConns = connections.filter((c) => c.provider === provider);

  const pickProvider = (p: ForgeProvider) => {
    setProvider(p);
    if (p === "github" && mode !== "manual") setMode("manual");
  };

  const submit = async () => {
    setBusy(true);
    try {
      const input: RegisterForgeOAuthAppInput = {
        provider,
        forge_base_url: baseURL.trim() || undefined,
        mode,
      };
      if (mode === "manual") {
        input.client_id = clientID.trim();
        input.client_secret = clientSecret.trim();
      } else if (mode === "auto") {
        input.admin_token = adminToken.trim();
      } else {
        input.connection_id = connectionID;
      }
      await registerForgeOAuthApp(teamID, input);
      setAdminToken("");
      setConnectionID("");
      setClientID("");
      setClientSecret("");
      setBaseURL("");
      setShow(false);
      onRegistered();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // GitHub has no create-app API: instead iterion hands GitHub a pre-filled App
  // manifest the browser POSTs; GitHub creates the App and redirects back to
  // iterion's callback, which stores the credentials. One click, no admin token.
  const launchGitHubManifest = async () => {
    setBusy(true);
    try {
      const { post_url, manifest } = await startGitHubManifest(teamID, {
        forge_base_url: baseURL.trim() || undefined,
        next: window.location.pathname + window.location.search,
      });
      const form = document.createElement("form");
      form.method = "POST";
      form.action = post_url;
      const field = document.createElement("input");
      field.type = "hidden";
      field.name = "manifest";
      field.value = JSON.stringify(manifest);
      form.appendChild(field);
      document.body.appendChild(form);
      form.submit(); // navigates to GitHub; the callback brings us back
    } catch (e) {
      onError((e as Error).message);
      setBusy(false);
    }
  };

  const canSubmit =
    mode === "manual"
      ? !!clientID.trim() && !!clientSecret.trim()
      : mode === "auto"
        ? autoSupported && !!adminToken.trim()
        : !!connectionID;

  if (!show) {
    return (
      <button
        type="button"
        onClick={() => setShow(true)}
        className="mt-3 text-accent-text hover:underline text-sm"
      >
        + Register an OAuth app
      </button>
    );
  }

  return (
    <section className="mt-3 bg-surface-1 border border-border-subtle rounded p-4 space-y-3">
      <h4 className="font-medium text-sm">Register an OAuth app</h4>
      <div className="flex gap-2 flex-wrap">
        {(["gitlab", "github", "forgejo"] as ForgeProvider[]).map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => pickProvider(p)}
            className={`text-sm rounded px-3 py-1 border ${
              provider === p ? "border-accent bg-surface-2" : "border-border-subtle"
            }`}
          >
            {p}
          </button>
        ))}
      </div>

      {provider === "github" && (
        <div className="rounded border border-accent/40 bg-accent/5 p-3 space-y-2">
          <Button
            variant="primary"
            onClick={() => void launchGitHubManifest()}
            disabled={busy}
            loading={busy}
          >
            {busy ? "Opening GitHub…" : "Create a GitHub App"}
          </Button>
          <p className="text-caption text-fg-muted">
            Recommended for GitHub — one click sends you to GitHub to confirm, then iterion stores
            the app's credentials automatically. (For GitHub Enterprise, set the base URL below
            first.) Or use the options below.
          </p>
        </div>
      )}

      <div className="flex flex-wrap gap-3 text-sm">
        <label
          className={`flex items-center gap-1 ${autoSupported ? "" : "opacity-50"}`}
          title={
            autoSupported ? "" : "GitHub auto-create needs the App-Manifest flow — paste credentials"
          }
        >
          <input
            type="radio"
            checked={mode === "auto"}
            onChange={() => setMode("auto")}
            disabled={!autoSupported}
          />
          Auto-create (admin token)
        </label>
        <label className="flex items-center gap-1">
          <input
            type="radio"
            checked={mode === "auto_from_connection"}
            onChange={() => setMode("auto_from_connection")}
          />
          Reuse a connection
        </label>
        <label className="flex items-center gap-1">
          <input type="radio" checked={mode === "manual"} onChange={() => setMode("manual")} />
          Paste credentials
        </label>
      </div>

      <div>
        <label htmlFor="oauth-app-base-url" className="sr-only">
          Forge base URL
        </label>
        <Input
          size="md"
          id="oauth-app-base-url"
          placeholder="Forge base URL (optional — for self-hosted, e.g. https://gitlab.example.com)"
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
        />
      </div>

      {mode === "auto" && (
        <>
          <div>
            <label htmlFor="oauth-admin-token" className="sr-only">
              Admin token
            </label>
            <Input
              size="md"
              type="password"
              id="oauth-admin-token"
              placeholder="Admin token (GitLab: instance-admin PAT with api scope)"
              value={adminToken}
              onChange={(e) => setAdminToken(e.target.value)}
              autoComplete="off"
            />
          </div>
          <p className="text-caption text-fg-muted">
            iterion creates the OAuth app on the forge for you (redirect URI + scope set
            automatically) and stores its credentials sealed. The admin token is used once and never
            stored.
          </p>
        </>
      )}

      {mode === "auto_from_connection" && (
        <>
          <div>
            <label htmlFor="oauth-conn-pick" className="sr-only">
              Connection
            </label>
            <Select
              size="md"
              id="oauth-conn-pick"
              value={connectionID}
              onChange={(e) => setConnectionID(e.target.value)}
            >
              <option value="">Select a {provider} connection…</option>
              {usableConns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.account_login ?? c.id} · {c.forge_base_url ?? c.provider}
                </option>
              ))}
            </Select>
          </div>
          <p className="text-caption text-fg-muted">
            Reuses an existing {provider} connection's token to create the app — no admin token to
            paste. The connection's owner needs create-app rights on the instance.
          </p>
        </>
      )}

      {mode === "manual" && (
        <>
          <div>
            <label htmlFor="oauth-client-id" className="sr-only">
              Client ID
            </label>
            <Input
              size="md"
              id="oauth-client-id"
              placeholder="Client ID (Application ID)"
              value={clientID}
              onChange={(e) => setClientID(e.target.value)}
              autoComplete="off"
            />
          </div>
          <div>
            <label htmlFor="oauth-client-secret" className="sr-only">
              Client secret
            </label>
            <Input
              size="md"
              type="password"
              id="oauth-client-secret"
              placeholder="Client secret"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              autoComplete="off"
            />
          </div>
          <p className="text-caption text-fg-muted">
            Create the app on the forge with redirect URI{" "}
            <span className="font-mono break-all">{redirectURI}</span> and the scope it needs
            (GitLab: <span className="font-mono">api</span>), then paste its credentials here.
          </p>
        </>
      )}

      <div className="flex items-center gap-2">
        <Button
          variant="primary"
          onClick={() => void submit()}
          disabled={busy || !canSubmit}
          loading={busy}
        >
          {busy ? "Registering…" : "Register"}
        </Button>
        <Button variant="ghost" onClick={() => setShow(false)}>
          Cancel
        </Button>
      </div>
    </section>
  );
}
