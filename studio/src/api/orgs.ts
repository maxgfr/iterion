// Super-admin org (team) console — REST client.
// Mirrors pkg/server/admin_orgs_routes.go. "org" is the public alias
// for the internal Team/tenant.

import { request } from "./client";

export interface OrgView {
  id: string;
  name: string;
  slug: string;
  status: string; // active | suspended | read_only
  personal?: boolean;
  monthly_run_quota?: number;
  memory_quota_bytes?: number;
  suspend_reason?: string;
  created_at?: string;
}

export interface OrgUsage {
  org: OrgView;
  members: number;
  effective_memory_quota_bytes: number;
  monthly_run_quota: number;
}

export async function listOrgs(): Promise<OrgView[]> {
  const res = await request<{ orgs: OrgView[] }>("/admin/orgs");
  return res.orgs ?? [];
}

export async function createOrg(input: {
  name: string;
  slug?: string;
  owner_email?: string;
}): Promise<OrgView> {
  return request<OrgView>("/admin/orgs", { method: "POST", body: JSON.stringify(input) });
}

export async function getOrgUsage(id: string): Promise<OrgUsage> {
  return request<OrgUsage>(`/admin/orgs/${encodeURIComponent(id)}/usage`);
}

export async function updateOrg(
  id: string,
  patch: { name?: string; slug?: string; monthly_run_quota?: number; memory_quota_bytes?: number },
): Promise<OrgView> {
  return request<OrgView>(`/admin/orgs/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export async function setOrgStatus(id: string, status: string, reason?: string): Promise<OrgView> {
  return request<OrgView>(`/admin/orgs/${encodeURIComponent(id)}/status`, {
    method: "POST",
    body: JSON.stringify({ status, reason }),
  });
}

const GiB = 1 << 30;

/** Render a byte quota as a compact GiB string. */
export function fmtQuotaGiB(bytes?: number): string {
  if (!bytes || bytes <= 0) return "default";
  return `${(bytes / GiB).toFixed(bytes % GiB === 0 ? 0 : 1)} GiB`;
}

/** Convert a GiB number to bytes for the API. */
export function gibToBytes(gib: number): number {
  return Math.round(gib * GiB);
}
