import { create } from "zustand";

import { readJSONFlag, writeJSONFlag } from "@/lib/localStorageFlag";

// Downloads history for the Artifacts panel. Persisted to localStorage
// so the list survives a SPA reload (matching browsers' built-in
// download manager). Pure metadata: the file bytes are not stored —
// "Show" / "Re-download" actions refetch from the run's artifact-files
// endpoint, which is the source of truth.

const STORAGE_KEY = "iterion.downloads.v1";
const MAX_ENTRIES = 50;

export interface DownloadEntry {
  id: string;
  runId: string;
  // path is relative to the run's artifact_files dir (matches
  // ArtifactFile.path from the API).
  path: string;
  basename: string;
  size: number;
  contentType: string;
  // Epoch milliseconds.
  downloadedAt: number;
  // Set in desktop mode when SaveBinaryFile returned a chosen path.
  // Undefined in browser mode (the SPA has no insight into where the
  // browser saved the file). When set, "Reveal in folder" is offered.
  localPath?: string;
}

function readEntries(): DownloadEntry[] {
  const parsed = readJSONFlag<unknown>(STORAGE_KEY, []);
  if (!Array.isArray(parsed)) return [];
  return parsed.filter(isEntry).slice(0, MAX_ENTRIES);
}

function writeEntries(list: DownloadEntry[]) {
  writeJSONFlag(STORAGE_KEY, list);
}

function isEntry(v: unknown): v is DownloadEntry {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return (
    typeof o.id === "string" &&
    typeof o.runId === "string" &&
    typeof o.path === "string" &&
    typeof o.basename === "string" &&
    typeof o.size === "number" &&
    typeof o.contentType === "string" &&
    typeof o.downloadedAt === "number"
  );
}

interface DownloadsState {
  entries: DownloadEntry[];
  recordDownload: (entry: Omit<DownloadEntry, "id" | "downloadedAt"> & { downloadedAt?: number }) => DownloadEntry;
  removeDownload: (id: string) => void;
  clearForRun: (runId: string) => void;
  clearAll: () => void;
}

let nextID = 0;
function newID(): string {
  nextID += 1;
  return `${Date.now().toString(36)}-${nextID.toString(36)}`;
}

export const useDownloadsStore = create<DownloadsState>((set) => ({
  entries: readEntries(),
  recordDownload: (input) => {
    const entry: DownloadEntry = {
      id: newID(),
      downloadedAt: input.downloadedAt ?? Date.now(),
      runId: input.runId,
      path: input.path,
      basename: input.basename,
      size: input.size,
      contentType: input.contentType,
      localPath: input.localPath,
    };
    set((s) => {
      // Most-recent first, capped to MAX_ENTRIES.
      const next = [entry, ...s.entries].slice(0, MAX_ENTRIES);
      writeEntries(next);
      return { entries: next };
    });
    return entry;
  },
  removeDownload: (id) =>
    set((s) => {
      const next = s.entries.filter((e) => e.id !== id);
      writeEntries(next);
      return { entries: next };
    }),
  clearForRun: (runId) =>
    set((s) => {
      const next = s.entries.filter((e) => e.runId !== runId);
      writeEntries(next);
      return { entries: next };
    }),
  clearAll: () => {
    writeEntries([]);
    set({ entries: [] });
  },
}));
