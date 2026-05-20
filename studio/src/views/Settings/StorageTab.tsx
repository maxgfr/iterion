import { useServerInfoStore } from "@/store/serverInfo";

export default function StorageTab() {
  const info = useServerInfoStore((s) => s.info);
  if (!info) return <p className="text-fg-subtle p-4">Loading…</p>;
  return (
    <div className="flex flex-col gap-3 p-4 text-sm">
      <div>
        <div className="font-medium text-fg-default mb-1">Working directory</div>
        <p className="text-xs text-fg-subtle mb-2">
          Absolute path the server was launched against. Runs, artifacts, and the
          native kanban store live under it.
        </p>
        <pre className="bg-surface-2 rounded p-2 text-xs font-mono overflow-x-auto">
          {info.work_dir || <span className="text-fg-subtle">(none — cloud mode)</span>}
        </pre>
      </div>
      {info.project_name && (
        <div>
          <div className="font-medium text-fg-default mb-1">Project</div>
          <div className="text-xs">{info.project_name}</div>
        </div>
      )}
      <div>
        <div className="font-medium text-fg-default mb-1">Mode</div>
        <div className="text-xs">{info.mode}</div>
      </div>
    </div>
  );
}
