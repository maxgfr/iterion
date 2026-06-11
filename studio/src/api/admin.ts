// Super-admin user console — REST client. Mirrors auth_routes.go's
// /api/admin/users handlers.

import { request } from "./client";
import type { UserStatus, UserView } from "./auth";
import { FeatureUnavailableError } from "./webhooks";

export { FeatureUnavailableError };

export interface AdminUsersResponse {
  users: UserView[];
  offset: number;
  limit: number;
}

export interface AdminListUsersQuery {
  offset?: number;
  limit?: number;
}

export interface AdminUpdateUserInput {
  status?: UserStatus;
  is_super_admin?: boolean;
  name?: string;
}

async function guard<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (err) {
    if (err instanceof Error && /API error 404:/.test(err.message)) {
      throw new FeatureUnavailableError("admin-users", err.message);
    }
    throw err;
  }
}

export function listAdminUsers(q: AdminListUsersQuery = {}): Promise<AdminUsersResponse> {
  const sp = new URLSearchParams();
  if (q.offset && q.offset > 0) sp.set("offset", String(q.offset));
  if (q.limit && q.limit > 0) sp.set("limit", String(q.limit));
  const s = sp.toString();
  return guard(() =>
    request<AdminUsersResponse>(`/admin/users${s ? `?${s}` : ""}`),
  );
}

export function updateAdminUser(
  userID: string,
  input: AdminUpdateUserInput,
): Promise<UserView> {
  return guard(() =>
    request<UserView>(`/admin/users/${encodeURIComponent(userID)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  );
}
