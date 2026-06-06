// Bot registry — REST client. Mirrors pkg/server/bots_routes.go.
// All paths are relative to the studio's same-origin server.

import { apiRequest } from "./client";
import type { PresetsBlock, VarsBlock } from "./types";

const BASE = "/api/v1/bots";

// ---------------------------------------------------------------------------
// Types — mirror pkg/botregistry JSON tags
// ---------------------------------------------------------------------------

/** BotEntry is the metadata-only shape returned by the registry list
 *  endpoint and embedded inside BotEntryWithSchema. */
export interface BotEntry {
  name: string;
  /** Bundle persona from manifest.yaml `display_name` (e.g. "Nexie",
   *  "Revi"). Empty for loose .bot files / un-personified bundles. The
   *  studio shows it as the lead label with `name` as a muted aside. */
  display_name?: string;
  description?: string;
  path: string;
  triggers?: string[];
  capabilities?: string[];
  /** Orchestrator-facing "use when" guidance (manifest when_to_use) that
   *  Nexie reads to route a task. Editable in the Bot metadata panel. */
  when_to_use?: string;
  /** Resolved catalog visibility: manifest `enabled` default composed
   *  with the workspace overlay. `false` = hidden from Nexie + the board
   *  picker (but still listed in the Catalog manager to flip back on).
   *  Absent is treated as enabled. */
  enabled?: boolean;
  /** True when this entry is a bundle (manifest.yaml + main.bot) and thus
   *  has metadata that can be edited; loose .bot files are read-only. */
  is_bundle?: boolean;
  /** Manifest author/version, surfaced so the Bot metadata panel can
   *  pre-fill + edit them. */
  author?: string;
  version?: string;
  /** The manifest `enabled` DEFAULT (pre-overlay). The Bot panel edits
   *  this; `enabled` is the resolved value the Catalog manager overlay
   *  controls. They differ when a workspace overlay is active. */
  manifest_enabled?: boolean;
}

/** BotPatch is the editable subset of a bot's manifest. Omitted fields
 *  are left untouched server-side; an empty string clears a field. */
export type BotPatch = Partial<{
  display_name: string;
  description: string;
  author: string;
  version: string;
  when_to_use: string;
  enabled: boolean;
  triggers: string[];
}>;

/** BotEntryWithSchema augments BotEntry with the workflow's declared
 *  vars + presets — same JSON shape as the studio's existing
 *  VarsBlock/PresetsBlock so VarFieldInput consumes it unchanged. */
export interface BotEntryWithSchema extends BotEntry {
  vars?: VarsBlock;
  presets?: PresetsBlock;
  /** Non-empty when the bot's source failed to parse. The picker still
   *  shows the bot but the typed form is hidden / surfaces an error. */
  schema_error?: string;
}

interface ListResponse {
  bots: BotEntryWithSchema[];
}

// ---------------------------------------------------------------------------
// REST surface
// ---------------------------------------------------------------------------

/** listBots returns every bot the host knows about along with its
 *  vars/presets schema. The full schemas are bundled in the list
 *  payload so the picker can switch bots without a second round trip. */
export async function listBots(): Promise<BotEntryWithSchema[]> {
  const r = await apiRequest<ListResponse>(BASE);
  return r.bots ?? [];
}

/** getBot fetches a single bot by name with its full schema. Useful
 *  when a ticket references a bot the list endpoint hasn't loaded
 *  yet (cache miss / page reload while a modal is open). */
export function getBot(name: string): Promise<BotEntryWithSchema> {
  return apiRequest<BotEntryWithSchema>(`${BASE}/${encodeURIComponent(name)}`);
}

/** updateBot writes a bot's manifest metadata (Bot metadata panel) and
 *  returns the refreshed entry. Bundle-only — the server rejects loose
 *  .bot files with 409. */
export function updateBot(name: string, patch: BotPatch): Promise<BotEntryWithSchema> {
  return apiRequest<BotEntryWithSchema>(`${BASE}/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

/** setBotOverlay pins a bot's catalog visibility in this workspace
 *  without touching the (possibly git-tracked) manifest — the Catalog
 *  manager quick-toggle. `null` clears the override (manifest default
 *  stands again). Returns the refreshed entry. */
export function setBotOverlay(name: string, enabled: boolean | null): Promise<BotEntryWithSchema> {
  return apiRequest<BotEntryWithSchema>(`${BASE}/${encodeURIComponent(name)}/overlay`, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}
