import { useEffect, useState } from "react";

import {
  FeatureUnavailableError,
  type MemoryDocumentMeta,
  type MemorySpaceRef,
  type MemoryVisibility,
  getMemoryUsage,
  listMemoryDocuments,
  memoryExportURL,
  readMemoryDocument,
} from "@/api/memory";
import { fmtBytes, pct } from "@/api/usage";

import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";

interface Props {
  teamID: string;
}

// All memory visibilities the server resolves. Some require extra
// qualifiers — we only expose those that make sense from a TeamPage:
// org-wide spaces by default, plus the global catalogue when the user
// wants to peek.
const VISIBILITIES: Array<{ id: MemoryVisibility; label: string; needsBot?: boolean }> = [
  { id: "org", label: "Org (shared across all bots)" },
  { id: "project", label: "Project (this workspace)" },
  { id: "bot", label: "Bot (per-bot scratch)", needsBot: true },
  { id: "user", label: "User (just me)" },
  { id: "global", label: "Global (instance-wide, read-only)" },
];

export default function MemoryTab({ teamID: _teamID }: Props) {
  const [ref, setRef] = useState<MemorySpaceRef>({ name: "default", visibility: "org" });
  const [usage, setUsage] = useState<{ used: number; quota: number } | null>(null);
  const [docs, setDocs] = useState<MemoryDocumentMeta[]>([]);
  const [selected, setSelected] = useState<MemoryDocumentMeta | null>(null);
  const [body, setBody] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  const reload = async () => {
    if (ref.visibility === "bot" && !ref.bot) {
      setErr("This visibility needs a bot id.");
      return;
    }
    setLoading(true);
    setErr(null);
    try {
      const [u, d] = await Promise.all([
        getMemoryUsage(ref).then((x) => ({ used: x.used_bytes, quota: x.quota_bytes })),
        listMemoryDocuments(ref),
      ]);
      setUsage(u);
      setDocs(d);
      setSelected(null);
      setBody("");
      setUnavailable(false);
    } catch (e) {
      if (e instanceof FeatureUnavailableError) setUnavailable(true);
      else setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const open = async (d: MemoryDocumentMeta) => {
    setSelected(d);
    setBody("");
    try {
      const txt = await readMemoryDocument(ref, d.path);
      setBody(txt);
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  if (unavailable) {
    return (
      <EmptyState
        title="Memory browser not enabled on this server"
        message="The shared-memory REST surface requires the cloud-mode knowledge store."
      />
    );
  }

  const cur = VISIBILITIES.find((v) => v.id === ref.visibility);

  return (
    <div className="space-y-4">
      {err && (
        <div className="text-sm text-fg-error bg-surface-warn-subtle border border-border-warn rounded px-3 py-2">
          {err}
        </div>
      )}

      <section className="grid grid-cols-1 sm:grid-cols-4 gap-2 items-end">
        <label className="block text-xs">
          <div className="text-fg-muted mb-1">Visibility</div>
          <Select
            value={ref.visibility}
            onChange={(e) =>
              setRef((r) => ({ ...r, visibility: e.target.value as MemoryVisibility }))
            }
          >
            {VISIBILITIES.map((v) => (
              <option key={v.id} value={v.id}>
                {v.label}
              </option>
            ))}
          </Select>
        </label>
        <label className="block text-xs">
          <div className="text-fg-muted mb-1">Space name</div>
          <Input
            value={ref.name}
            onChange={(e) => setRef((r) => ({ ...r, name: e.target.value }))}
            placeholder="default"
          />
        </label>
        {cur?.needsBot && (
          <label className="block text-xs">
            <div className="text-fg-muted mb-1">Bot id</div>
            <Input
              value={ref.bot ?? ""}
              onChange={(e) => setRef((r) => ({ ...r, bot: e.target.value }))}
              placeholder="bot-name"
            />
          </label>
        )}
        <label className="block text-xs">
          <div className="text-fg-muted mb-1">Project (optional)</div>
          <Input
            value={ref.project ?? ""}
            onChange={(e) => setRef((r) => ({ ...r, project: e.target.value }))}
            placeholder=""
          />
        </label>
      </section>

      <div className="flex gap-2">
        <Button size="sm" variant="primary" loading={loading} onClick={() => void reload()}>
          Load
        </Button>
        <a
          href={memoryExportURL(ref)}
          className="inline-flex items-center text-xs text-fg-muted hover:text-fg-default underline"
        >
          Export space (.tar.gz)
        </a>
      </div>

      {usage && (
        <div className="text-xs space-y-1 bg-surface-1 border border-border-subtle rounded p-3">
          <div className="text-fg-muted">Storage</div>
          <div className="flex items-baseline gap-1">
            <span className="text-lg font-semibold">{fmtBytes(usage.used)}</span>
            {usage.quota > 0 && (
              <span className="text-fg-muted">/ {fmtBytes(usage.quota)}</span>
            )}
          </div>
          {usage.quota > 0 && (
            <div className="h-1.5 bg-surface-2 rounded overflow-hidden">
              <div
                className="h-full bg-accent"
                style={{ width: `${pct(usage.used, usage.quota) ?? 0}%` }}
              />
            </div>
          )}
        </div>
      )}

      <section className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div>
          <h3 className="font-medium mb-2">Documents</h3>
          {docs.length === 0 ? (
            <EmptyState message="No documents in this space yet." />
          ) : (
            <ul className="divide-y divide-border-subtle bg-surface-1 border border-border-subtle rounded">
              {docs.map((d) => (
                <li
                  key={d.path}
                  className={`px-2 py-1.5 text-sm cursor-pointer hover:bg-surface-2 ${
                    selected?.path === d.path ? "bg-surface-2" : ""
                  }`}
                  onClick={() => void open(d)}
                >
                  <div className="font-mono text-xs">{d.path}</div>
                  <div className="text-xs text-fg-muted">
                    {fmtBytes(d.size)} ·{" "}
                    {d.updated_at ? new Date(d.updated_at).toLocaleString() : ""}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div>
          <h3 className="font-medium mb-2">
            Preview {selected ? <span className="font-mono text-xs">{selected.path}</span> : null}
          </h3>
          {selected ? (
            <pre className="bg-surface-1 border border-border-subtle rounded p-3 text-xs whitespace-pre-wrap break-words max-h-[60vh] overflow-auto">
              {body || "Loading…"}
            </pre>
          ) : (
            <EmptyState message="Pick a document on the left to preview it." />
          )}
        </div>
      </section>
    </div>
  );
}
