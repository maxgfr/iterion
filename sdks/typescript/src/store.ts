/**
 * Direct readers for the iterion run store.
 *
 * The store is a documented public format (`docs/persisted-formats.md`),
 * so reading it without going through the CLI is supported and is
 * notably faster for repeated lookups (events tail, artifact diffing).
 */

import { readdir } from "node:fs/promises";
import { join } from "node:path";

import { IterionStoreParseError } from "./errors.js";
import {
  artifactDir,
  interactionPath,
  readJSON,
  resolveStoreDir,
  runJsonPath,
} from "./paths.js";
import type { Artifact, Interaction, Run } from "./types.js";

function isFileNotFound(err: unknown): boolean {
  return (
    typeof err === "object" &&
    err !== null &&
    "code" in err &&
    (err as { code?: string }).code === "ENOENT"
  );
}

/** Read `<store>/runs/<runId>/run.json`. */
export async function loadRun(
  runId: string,
  opts: { storeDir?: string } = {},
): Promise<Run> {
  return readJSON<Run>(runJsonPath(runId, opts.storeDir));
}

/** Read `<store>/runs/<runId>/interactions/<interactionId>.json`. */
export async function loadInteraction(
  runId: string,
  interactionId: string,
  opts: { storeDir?: string } = {},
): Promise<Interaction> {
  return readJSON<Interaction>(interactionPath(runId, interactionId, opts.storeDir));
}

/**
 * Read an artifact for a given node. When `version` is omitted, the
 * latest version (per `run.json`'s `artifact_index`, falling back to a
 * directory scan) is returned.
 */
export async function loadArtifact(
  runId: string,
  nodeId: string,
  version?: number,
  opts: { storeDir?: string } = {},
): Promise<Artifact> {
  const dir = artifactDir(runId, nodeId, opts.storeDir);
  let v = version;
  if (v === undefined) {
    try {
      const run = await loadRun(runId, opts);
      const idx = run.artifact_index?.[nodeId];
      if (typeof idx === "number") {
        v = idx;
      }
    } catch (err) {
      // A missing run.json is expected for some flows (e.g. external SDK
      // consumers reading mid-run); fall through to a directory scan. But a
      // *parse* error means the file is there and corrupt — surface it so the
      // caller does not silently mask a broken store.
      if (err instanceof IterionStoreParseError) throw err;
      if (!isFileNotFound(err)) throw err;
    }
  }
  if (v === undefined) {
    const entries = await readdir(dir);
    const versions = entries
      .map((e) => Number(e.replace(/\.json$/, "")))
      .filter((n) => Number.isFinite(n));
    if (versions.length === 0) {
      throw new Error(`no artifacts found for node ${nodeId} in run ${runId}`);
    }
    v = Math.max(...versions);
  }
  return readJSON<Artifact>(join(dir, `${v}.json`));
}

/** List run IDs present in the store. */
export async function listRuns(
  opts: { storeDir?: string } = {},
): Promise<string[]> {
  const dir = join(resolveStoreDir(opts.storeDir), "runs");
  try {
    const entries = await readdir(dir, { withFileTypes: true });
    return entries.filter((e) => e.isDirectory()).map((e) => e.name);
  } catch {
    return [];
  }
}
