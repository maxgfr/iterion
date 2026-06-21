// Extracted from api/runs.ts to keep that file focused.
// Merge conflict resolution endpoints feeding the MergeConflictView:
// list/refresh, manual resolve, agent-assisted resolve, finalize, abort.

import { request } from "./client";
import type {
  FinalizeMergeConflictRequest,
  MergeConflictsResponse,
  MergeRunResponse,
  ResolveMergeConflictRequest,
  ResolveWithAgentRequest,
} from "./types";

export async function getMergeConflicts(
  runId: string,
): Promise<MergeConflictsResponse> {
  return request(`/runs/${encodeURIComponent(runId)}/merge/conflicts`);
}

export async function resolveMergeConflict(
  runId: string,
  req: ResolveMergeConflictRequest,
): Promise<MergeConflictsResponse> {
  return request(
    `/runs/${encodeURIComponent(runId)}/merge/conflicts/resolve`,
    { method: "POST", body: JSON.stringify(req) },
  );
}

export async function resolveMergeConflictWithAgent(
  runId: string,
  req: ResolveWithAgentRequest = {},
): Promise<MergeConflictsResponse> {
  return request(
    `/runs/${encodeURIComponent(runId)}/merge/conflicts/resolve-with-agent`,
    { method: "POST", body: JSON.stringify(req) },
  );
}

export async function finalizeMergeConflict(
  runId: string,
  req: FinalizeMergeConflictRequest = {},
): Promise<MergeRunResponse> {
  return request(
    `/runs/${encodeURIComponent(runId)}/merge/conflicts/finalize`,
    { method: "POST", body: JSON.stringify(req) },
  );
}

export async function abortMergeConflict(runId: string): Promise<void> {
  await request(`/runs/${encodeURIComponent(runId)}/merge/conflicts/abort`, {
    method: "POST",
  });
}
