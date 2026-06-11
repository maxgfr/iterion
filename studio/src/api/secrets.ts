// Generic-secret REST client. Mirrors pkg/server/generic_secrets_routes.go.
// Two scopes: team (org admin) and personal (`/me`).

import { request } from "./client";
import { FeatureUnavailableError } from "./webhooks";

export { FeatureUnavailableError };

export interface GenericSecretView {
  id: string;
  name: string;
  last4?: string;
  fingerprint?: string;
  scope_user_id?: string;
  created_at: string;
  last_used_at?: string;
}

export interface CreateSecretInput {
  name: string;
  secret: string;
}

export interface UpdateSecretInput {
  name?: string;
  secret?: string;
}

async function guard<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (err) {
    if (err instanceof Error && /API error 404:/.test(err.message)) {
      throw new FeatureUnavailableError("secrets", err.message);
    }
    throw err;
  }
}

export async function listTeamSecrets(teamID: string): Promise<GenericSecretView[]> {
  return guard(async () => {
    const r = await request<{ secrets: GenericSecretView[] }>(
      `/teams/${encodeURIComponent(teamID)}/secrets`,
    );
    return r.secrets ?? [];
  });
}

export function createTeamSecret(
  teamID: string,
  input: CreateSecretInput,
): Promise<GenericSecretView> {
  return guard(() =>
    request<GenericSecretView>(`/teams/${encodeURIComponent(teamID)}/secrets`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  );
}

export function updateTeamSecret(
  teamID: string,
  secretID: string,
  input: UpdateSecretInput,
): Promise<GenericSecretView> {
  return guard(() =>
    request<GenericSecretView>(
      `/teams/${encodeURIComponent(teamID)}/secrets/${encodeURIComponent(secretID)}`,
      { method: "PATCH", body: JSON.stringify(input) },
    ),
  );
}

export async function deleteTeamSecret(teamID: string, secretID: string): Promise<void> {
  await guard(() =>
    request<void>(
      `/teams/${encodeURIComponent(teamID)}/secrets/${encodeURIComponent(secretID)}`,
      { method: "DELETE" },
    ),
  );
}

export async function listMySecrets(): Promise<GenericSecretView[]> {
  return guard(async () => {
    const r = await request<{ secrets: GenericSecretView[] }>(`/me/secrets`);
    return r.secrets ?? [];
  });
}

export function createMySecret(input: CreateSecretInput): Promise<GenericSecretView> {
  return guard(() =>
    request<GenericSecretView>(`/me/secrets`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  );
}

export function updateMySecret(
  secretID: string,
  input: UpdateSecretInput,
): Promise<GenericSecretView> {
  return guard(() =>
    request<GenericSecretView>(`/me/secrets/${encodeURIComponent(secretID)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  );
}

export async function deleteMySecret(secretID: string): Promise<void> {
  await guard(() =>
    request<void>(`/me/secrets/${encodeURIComponent(secretID)}`, { method: "DELETE" }),
  );
}

// ---- pure helpers (covered by vitest) ----

/**
 * isValidSecretName checks the server-enforced regex ^[A-Za-z_][A-Za-z0-9_]*$
 * with the same 128-byte cap. Returning an `error?` makes the form copy
 * actionable without re-implementing the rules inline.
 */
export function isValidSecretName(name: string): { ok: boolean; error?: string } {
  if (!name) return { ok: false, error: "name required" };
  if (name.length > 128) return { ok: false, error: "max 128 characters" };
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
    return {
      ok: false,
      error: "letters, digits and underscore only — must not start with a digit",
    };
  }
  return { ok: true };
}
