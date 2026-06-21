import { useState } from "react";

import { Dialog, EmptyState } from "@/components/ui";
import { useRunFiles } from "@/hooks/useRunFiles";
import type { RunFile } from "@/api/runs";
import FileDiffDialog from "./FileDiffDialog";

interface BranchDiffModalProps {
  runId: string;
  open: boolean;
  onClose: () => void;
}

// BranchDiffModal shows a run's full branch diff (BaseCommit..HEAD)
// without leaving the current view — used from the board's IssueModal
// so an operator can review a dispatcher-spawned run's changes inline
// instead of navigating to /runs/<id>'s Files tab. Lists the changed
// files; clicking one opens the shared Monaco FileDiffDialog in
// branch mode.
export default function BranchDiffModal({
  runId,
  open,
  onClose,
}: BranchDiffModalProps) {
  const { data, loading, error } = useRunFiles(open ? runId : null, "branch");
  const [selected, setSelected] = useState<RunFile | null>(null);

  const files = data?.files ?? [];
  const totalAdded = files.reduce((n, f) => n + Math.max(0, f.added), 0);
  const totalDeleted = files.reduce((n, f) => n + Math.max(0, f.deleted), 0);

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => !o && onClose()}
        title={
          <span className="flex items-baseline gap-2">
            Branch diff
            {files.length > 0 && (
              <span className="text-micro font-normal text-fg-subtle">
                {files.length} file{files.length === 1 ? "" : "s"} ·{" "}
                <span className="text-success-fg">+{totalAdded}</span>{" "}
                <span className="text-danger-fg">−{totalDeleted}</span>
              </span>
            )}
          </span>
        }
        widthClass="max-w-2xl"
      >
        <div className="max-h-[60vh] overflow-y-auto">
          {error ? (
            <EmptyState message={error} />
          ) : loading && !data ? (
            <EmptyState message="Loading diff…" />
          ) : !data?.available ? (
            <EmptyState message={branchUnavailableLabel(data?.reason)} />
          ) : files.length === 0 ? (
            <EmptyState message="No changes on this branch." />
          ) : (
            <ul className="flex flex-col">
              {files.map((f) => (
                <li key={f.path}>
                  <button
                    type="button"
                    onClick={() => setSelected(f)}
                    className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-xs hover:bg-surface-2 focus:bg-surface-2 focus:outline-none"
                  >
                    <StatusGlyph status={f.status} />
                    <span className="flex-1 min-w-0 truncate font-mono" title={f.path}>
                      {f.path}
                    </span>
                    {f.binary ? (
                      <span className="text-caption text-fg-subtle shrink-0">(binary)</span>
                    ) : (
                      <span className="text-caption shrink-0 tabular-nums">
                        <span className="text-success-fg">+{Math.max(0, f.added)}</span>{" "}
                        <span className="text-danger-fg">−{Math.max(0, f.deleted)}</span>
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Dialog>
      <FileDiffDialog
        runId={runId}
        file={selected}
        mode="branch"
        onClose={() => setSelected(null)}
      />
    </>
  );
}

function StatusGlyph({ status }: { status: string }) {
  const { ch, cls } = statusMeta(status);
  return (
    <span
      className={`shrink-0 w-4 text-center font-mono text-micro ${cls}`}
      title={statusTitle(status)}
    >
      {ch}
    </span>
  );
}

function statusMeta(status: string): { ch: string; cls: string } {
  switch (status) {
    case "A":
      return { ch: "A", cls: "text-success-fg" };
    case "D":
      return { ch: "D", cls: "text-danger-fg" };
    case "R":
      return { ch: "R", cls: "text-info-fg" };
    case "??":
      return { ch: "?", cls: "text-fg-subtle" };
    case "M":
    default:
      return { ch: "M", cls: "text-warning-fg" };
  }
}

function statusTitle(status: string): string {
  switch (status) {
    case "A":
      return "Added";
    case "D":
      return "Deleted";
    case "R":
      return "Renamed";
    case "??":
      return "Untracked";
    case "M":
      return "Modified";
    default:
      return status;
  }
}

function branchUnavailableLabel(reason: string | undefined): string {
  switch (reason) {
    case "no_baseline":
      return "Run has no base commit — branch diff unavailable.";
    case "no_workdir":
      return "No working directory recorded for this run.";
    case "not_git_repo":
      return "Not a git repository.";
    default:
      return "Branch diff unavailable.";
  }
}
