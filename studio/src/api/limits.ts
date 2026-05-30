import { apiRequest } from "./client";
import type { CostCapStatus } from "./types";

// getCostCapStatus fetches the current per-(store, UTC-day) spend-cap
// status. A disabled cap reports { enabled: false }.
export async function getCostCapStatus(): Promise<CostCapStatus> {
  return apiRequest<CostCapStatus>("/api/v1/limits/cost");
}

// overrideCostCapForToday grants (or revokes) the daily spend-cap
// override for the current UTC day. A bare call grants it; pass
// active=false to revoke. The grant is recorded server-side as an audit
// trail and auto-clears at the next UTC day.
export async function overrideCostCapForToday(
  opts: { active?: boolean; note?: string } = {},
): Promise<CostCapStatus> {
  return apiRequest<CostCapStatus>("/api/v1/limits/cost/override", {
    method: "POST",
    body: JSON.stringify({ active: opts.active ?? true, note: opts.note }),
  });
}
