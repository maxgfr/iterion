// Tiny wrappers around localStorage to remember the current whats-next
// run id per project. Survives a page reload so reopening /whats-next
// resumes the in-flight conversation instead of presenting a fresh
// launcher.
//
// Keyed by (botId, projectId) so different first-class bots (when we
// add more) and different projects don't trample each other.

import { readStringFlag, writeStringFlag, removeFlag } from "../localStorageFlag";

const KEY_PREFIX = "iterion.whats-next.runId";

function key(botId: string, projectId: string | null | undefined): string {
  return `${KEY_PREFIX}.${botId}.${projectId ?? "_default"}`;
}

export function rememberSessionRunId(
  botId: string,
  projectId: string | null | undefined,
  runId: string,
): void {
  writeStringFlag(key(botId, projectId), runId);
}

export function recallSessionRunId(
  botId: string,
  projectId: string | null | undefined,
): string | null {
  const raw = readStringFlag(key(botId, projectId));
  return raw.length > 0 ? raw : null;
}

export function forgetSessionRunId(
  botId: string,
  projectId: string | null | undefined,
): void {
  removeFlag(key(botId, projectId));
}
