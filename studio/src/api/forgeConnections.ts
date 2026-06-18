import { guard404, request } from "./client";

// Mirrors pkg/forge: the OUTBOUND forge-integration layer (connect a repo +
// auto-provision the inbound webhook + token binding). Distinct from
// api/webhooks.ts (the raw inbound-webhook CRUD).

export type ForgeProvider = "gitlab" | "github" | "forgejo";
export type ForgeKind = "oauth_app" | "github_app" | "pat";
export type ForgeConnectionStatus = "active" | "needs_reauth" | "revoked";

export interface ForgeConnection {
  id: string;
  tenant_id: string;
  provider: ForgeProvider;
  kind: ForgeKind;
  display_name?: string;
  forge_base_url?: string;
  account_login?: string;
  account_id?: string;
  namespace?: string;
  status: ForgeConnectionStatus;
  access_token_expires_at?: string;
  scopes?: string[];
  managed_secret_id?: string;
  created_at: string;
}

export interface ForgeRepo {
  full_name: string;
  description?: string;
  private: boolean;
  default_branch?: string;
  web_url?: string;
  can_admin: boolean;
}

export interface ForgeIntegration {
  id: string;
  connection_id: string;
  provider: ForgeProvider;
  repo_full_name: string;
  bot_ids: string[];
  events_normalized: string[];
  webhook_id: string;
  hook_id: string;
  hook_url?: string;
  managed_secret_id?: string;
  created_at: string;
}

export interface ForgeEnablePreview {
  events_normalized: string[];
  /** The forge's native event names the hook will subscribe to. */
  forge_native_events: string[];
  scopes: Record<string, string>;
  secrets: Array<{ bot_id: string; secret: string }>;
  identity: { handle: string; provider: string; base_url: string };
  /** Non-empty = a bot can't be auto-installed (no forge: block / not found). */
  conflicts: string[];
}

export interface ForgeProvisionResult {
  integration_id: string;
  webhook_id: string;
  hook_id: string;
  managed_secret_id: string;
  bot_ids: string[];
  created: boolean;
}

export interface ConnectForgeInput {
  provider: ForgeProvider;
  mode: "oauth" | "pat";
  forge_base_url?: string;
  pat?: string;
  display_name?: string;
  /** Studio path to return to after an OAuth round-trip. */
  next?: string;
}

export interface ConnectForgeResult {
  connection?: ForgeConnection;
  /** Present for mode=oauth — the studio redirects the window here. */
  authorize_url?: string;
}

export async function listForgeConnections(teamID: string): Promise<ForgeConnection[]> {
  const r = await guard404("forge_integrations", () =>
    request<{ connections: ForgeConnection[] }>(`/teams/${teamID}/forge/connections`),
  );
  return r.connections ?? [];
}

export async function connectForge(
  teamID: string,
  input: ConnectForgeInput,
): Promise<ConnectForgeResult> {
  return request(`/teams/${teamID}/forge/connections`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function deleteForgeConnection(teamID: string, connID: string): Promise<void> {
  await request<void>(`/teams/${teamID}/forge/connections/${connID}`, { method: "DELETE" });
}

export async function listForgeRepos(
  teamID: string,
  connID: string,
  search?: string,
  page?: number,
): Promise<ForgeRepo[]> {
  const params = new URLSearchParams();
  if (search) params.set("search", search);
  if (page) params.set("page", String(page));
  const qs = params.toString() ? `?${params.toString()}` : "";
  const r = await request<{ repos: ForgeRepo[] }>(
    `/teams/${teamID}/forge/connections/${connID}/repos${qs}`,
  );
  return r.repos ?? [];
}

export async function listForgeIntegrations(teamID: string): Promise<ForgeIntegration[]> {
  const r = await guard404("forge_integrations", () =>
    request<{ integrations: ForgeIntegration[] }>(`/teams/${teamID}/forge/repo-bots`),
  );
  return r.integrations ?? [];
}

export async function previewForgeEnable(
  teamID: string,
  connID: string,
  repo: string,
  bots: string[],
): Promise<ForgeEnablePreview> {
  const params = new URLSearchParams({ connection_id: connID, repo, bots: bots.join(",") });
  return request(`/teams/${teamID}/forge/repo-bots/preview?${params.toString()}`);
}

export async function enableForgeRepoBots(
  teamID: string,
  connID: string,
  repo: string,
  botIDs: string[],
): Promise<ForgeProvisionResult> {
  return request(`/teams/${teamID}/forge/repo-bots`, {
    method: "POST",
    body: JSON.stringify({ connection_id: connID, repo, bot_ids: botIDs }),
  });
}

export async function disableForgeIntegration(
  teamID: string,
  integrationID: string,
): Promise<void> {
  await request<void>(`/teams/${teamID}/forge/repo-bots/${integrationID}`, { method: "DELETE" });
}
