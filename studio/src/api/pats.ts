// Personal Access Token REST client. Mirrors pkg/server/pat_routes.go.
// URL space is /api/me/tokens — deliberately distinct from /api/me/api-keys
// (BYOK LLM provider keys).

import { FeatureUnavailableError, guard404, request } from "./client";

export { FeatureUnavailableError };

export interface PersonalAccessToken {
  id: string;
  user_id: string;
  name: string;
  token_last4: string;
  fingerprint?: string;
  team_id?: string;
  created_at: string;
  expires_at?: string;
  last_used_at?: string;
  revoked_at?: string;
}

export interface CreatePATInput {
  name: string;
  team_id?: string;
  expires_in_days?: number;
}

export interface CreatePATResponse {
  pat: PersonalAccessToken;
  // The plaintext shown ONCE — never re-fetchable.
  token: string;
}

export async function listMyTokens(): Promise<PersonalAccessToken[]> {
  return guard404("pats", async () => {
    const r = await request<{ tokens: PersonalAccessToken[] }>(`/me/tokens`);
    return r.tokens ?? [];
  });
}

export function createMyToken(input: CreatePATInput): Promise<CreatePATResponse> {
  return guard404("pats", () =>
    request<CreatePATResponse>(`/me/tokens`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  );
}

export async function revokeMyToken(tokenID: string): Promise<void> {
  await guard404("pats", () =>
    request<void>(`/me/tokens/${encodeURIComponent(tokenID)}`, { method: "DELETE" }),
  );
}
