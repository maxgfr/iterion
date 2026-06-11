// Bot-secret binding REST client. Mirrors pkg/server/bot_bindings_routes.go.

import { FeatureUnavailableError, guard404, request } from "./client";

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

export async function listBindings(
  teamID: string,
  botID: string,
): Promise<BotSecretBinding[]> {
  return guard404("bindings", async () => {
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
  return guard404("bindings", () =>
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
  return guard404("bindings", () =>
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
  await guard404("bindings", () =>
    request<void>(
      `/teams/${encodeURIComponent(teamID)}/bots/${encodeURIComponent(botID)}/bindings/${encodeURIComponent(bindingID)}`,
      { method: "DELETE" },
    ),
  );
}
