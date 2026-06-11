// Bot-secret binding REST client. Mirrors pkg/server/bot_bindings_routes.go.

import { request } from "./client";
import { FeatureUnavailableError } from "./webhooks";

export { FeatureUnavailableError };

export interface BotSecretBinding {
  id: string;
  tenant_id: string;
  bot_id: string;
  secret_id: string;
  secret_name_for_workflow: string;
  allowed_hosts?: string[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateBindingInput {
  secret_id: string;
  secret_name_for_workflow: string;
  allowed_hosts?: string[];
}

export interface UpdateBindingInput {
  secret_id?: string;
  secret_name_for_workflow?: string;
  allowed_hosts?: string[];
}

async function guard<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (err) {
    if (err instanceof Error && /API error 404:/.test(err.message)) {
      throw new FeatureUnavailableError("bindings", err.message);
    }
    throw err;
  }
}

export async function listBindings(
  teamID: string,
  botID: string,
): Promise<BotSecretBinding[]> {
  return guard(async () => {
    const r = await request<{ bindings: BotSecretBinding[] }>(
      `/teams/${encodeURIComponent(teamID)}/bots/${encodeURIComponent(botID)}/bindings`,
    );
    return r.bindings ?? [];
  });
}

export function createBinding(
  teamID: string,
  botID: string,
  input: CreateBindingInput,
): Promise<BotSecretBinding> {
  return guard(() =>
    request<BotSecretBinding>(
      `/teams/${encodeURIComponent(teamID)}/bots/${encodeURIComponent(botID)}/bindings`,
      { method: "POST", body: JSON.stringify(input) },
    ),
  );
}

export function updateBinding(
  teamID: string,
  botID: string,
  bindingID: string,
  input: UpdateBindingInput,
): Promise<BotSecretBinding> {
  return guard(() =>
    request<BotSecretBinding>(
      `/teams/${encodeURIComponent(teamID)}/bots/${encodeURIComponent(botID)}/bindings/${encodeURIComponent(bindingID)}`,
      { method: "PATCH", body: JSON.stringify(input) },
    ),
  );
}

export async function deleteBinding(
  teamID: string,
  botID: string,
  bindingID: string,
): Promise<void> {
  await guard(() =>
    request<void>(
      `/teams/${encodeURIComponent(teamID)}/bots/${encodeURIComponent(botID)}/bindings/${encodeURIComponent(bindingID)}`,
      { method: "DELETE" },
    ),
  );
}
