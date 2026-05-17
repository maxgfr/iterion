// Tiny wrappers around localStorage to remember the current pilote
// run id per project. Survives a page reload so reopening /pilote
// resumes the in-flight conversation instead of presenting a fresh
// launcher.
//
// Keyed by (botId, projectId) so different first-class bots (when we
// add more) and different projects don't trample each other.

const KEY_PREFIX = "iterion.pilote.runId";

function key(botId: string, projectId: string | null | undefined): string {
  return `${KEY_PREFIX}.${botId}.${projectId ?? "_default"}`;
}

export function rememberSessionRunId(
  botId: string,
  projectId: string | null | undefined,
  runId: string,
): void {
  try {
    window.localStorage.setItem(key(botId, projectId), runId);
  } catch {
    // storage may be unavailable (private mode, quota); silently skip.
  }
}

export function recallSessionRunId(
  botId: string,
  projectId: string | null | undefined,
): string | null {
  try {
    const raw = window.localStorage.getItem(key(botId, projectId));
    return raw && raw.length > 0 ? raw : null;
  } catch {
    return null;
  }
}

export function forgetSessionRunId(
  botId: string,
  projectId: string | null | undefined,
): void {
  try {
    window.localStorage.removeItem(key(botId, projectId));
  } catch {
    // silently skip.
  }
}
