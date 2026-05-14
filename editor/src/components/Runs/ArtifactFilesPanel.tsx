import { useCallback, useEffect, useRef, useState } from "react";

import {
  artifactFileURL,
  listArtifactFiles,
  type ArtifactFile,
} from "@/api/runs";
import { useRunStore } from "@/store/run";

// Events that suggest a tool just dropped a new file. Refresh on
// node_finished (write_audit_md / emit_sbom complete here) and on the
// terminal triplet (run_finished + run_failed + run_cancelled) so the
// final SBOM shows up without the operator hitting refresh.
const REFRESH_EVENTS = new Set([
  "node_finished",
  "run_finished",
  "run_failed",
  "run_cancelled",
]);

const DEBOUNCE_MS = 300;

interface Props {
  runId: string | null;
}

// ArtifactFilesPanel surfaces the contents of runs/<id>/artifact_files
// — the per-run scratch area where in-sandbox tools (write_audit_md,
// emit_sbom, …) drop arbitrary report/SBOM/manifest files. This
// replaces the prior pattern of committing `docs/renovacy/*.md` into
// the bench repo (which leaked info + cluttered the operator's git
// history). Files are listed flat (paths can contain `/`); each row
// links to /api/runs/<id>/artifact-files/<path> for inline preview or
// download.
export default function ArtifactFilesPanel({ runId }: Props) {
  const [files, setFiles] = useState<ArtifactFile[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastSeenSeqRef = useRef<number>(-1);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const genRef = useRef(0);

  const events = useRunStore((s) => s.events);

  const fetchNow = useCallback(() => {
    if (!runId) return;
    const myGen = ++genRef.current;
    setLoading(true);
    listArtifactFiles(runId)
      .then((res) => {
        if (myGen !== genRef.current) return;
        setFiles(res);
        setError(null);
      })
      .catch((err: unknown) => {
        if (myGen !== genRef.current) return;
        setError(err instanceof Error ? err.message : "Failed to load files");
      })
      .finally(() => {
        if (myGen !== genRef.current) return;
        setLoading(false);
      });
  }, [runId]);

  // Initial fetch + refetch on run change.
  useEffect(() => {
    if (!runId) {
      setFiles(null);
      return;
    }
    fetchNow();
  }, [runId, fetchNow]);

  // Live refresh: when new events arrive that suggest a tool wrote a
  // file, schedule a debounced refetch. Tracking the last seen seq
  // avoids re-triggering on the same event after a re-render.
  useEffect(() => {
    if (!runId || events.length === 0) return;
    let touched = false;
    for (const ev of events) {
      if (ev.seq <= lastSeenSeqRef.current) continue;
      lastSeenSeqRef.current = ev.seq;
      if (REFRESH_EVENTS.has(ev.type)) {
        touched = true;
      }
    }
    if (!touched) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(fetchNow, DEBOUNCE_MS);
  }, [events, runId, fetchNow]);

  if (!runId) {
    return (
      <div className="h-full flex items-center justify-center text-fg-subtle text-xs px-4">
        No active run.
      </div>
    );
  }
  if (loading && files === null) {
    return (
      <div className="h-full flex items-center justify-center text-fg-subtle text-xs px-4">
        Loading artifacts…
      </div>
    );
  }
  if (error) {
    return (
      <div className="h-full overflow-auto px-4 py-3 text-xs">
        <div className="text-fg-error">Failed to load artifacts: {error}</div>
        <button
          type="button"
          className="mt-2 text-fg-link hover:underline"
          onClick={fetchNow}
        >
          Retry
        </button>
      </div>
    );
  }
  if (!files || files.length === 0) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-fg-subtle text-xs px-4 text-center gap-1">
        <div>No artifact files yet.</div>
        <div className="opacity-70">
          In-sandbox tools writing into{" "}
          <code className="px-1 rounded bg-surface-2">
            $ITERION_ARTIFACT_FILES_DIR
          </code>{" "}
          appear here as they land.
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto text-xs">
      <table className="w-full">
        <thead className="sticky top-0 bg-surface-1 border-b border-border-default">
          <tr className="text-left text-fg-subtle">
            <th className="px-3 py-2 font-normal">Path</th>
            <th className="px-3 py-2 font-normal text-right whitespace-nowrap">
              Size
            </th>
            <th className="px-3 py-2 font-normal whitespace-nowrap">
              Modified
            </th>
            <th className="px-3 py-2 font-normal text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {files.map((f) => {
            const url = artifactFileURL(runId, f.path);
            return (
              <tr
                key={f.path}
                className="border-b border-border-subtle hover:bg-surface-2"
              >
                <td className="px-3 py-1.5 font-mono">
                  <a
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-fg-link hover:underline"
                  >
                    {f.path}
                  </a>
                </td>
                <td className="px-3 py-1.5 text-right text-fg-subtle whitespace-nowrap">
                  {formatSize(f.size)}
                </td>
                <td className="px-3 py-1.5 text-fg-subtle whitespace-nowrap">
                  {formatModified(f.modified_at)}
                </td>
                <td className="px-3 py-1.5 text-right whitespace-nowrap">
                  <a
                    href={url}
                    download={basename(f.path)}
                    className="text-fg-link hover:underline mr-3"
                  >
                    download
                  </a>
                  <a
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-fg-link hover:underline"
                  >
                    open
                  </a>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function formatModified(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function basename(p: string): string {
  const i = p.lastIndexOf("/");
  return i < 0 ? p : p.slice(i + 1);
}
