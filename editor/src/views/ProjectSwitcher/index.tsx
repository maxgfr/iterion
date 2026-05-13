import { useEffect, useMemo, useState } from "react";
import { TrashIcon } from "@radix-ui/react-icons";

import { Button, Dialog, Input } from "@/components/ui";
import ConfirmDialog from "@/components/shared/ConfirmDialog";

import { useDesktop } from "@/hooks/useDesktop";
import type { Project } from "@/lib/desktopBridge";

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function ProjectSwitcher({ open, onClose }: Props) {
  const {
    projects,
    currentProject,
    switchProject,
    pickAndAddProject,
    removeProject,
  } = useDesktop();
  const [query, setQuery] = useState("");
  const [pendingRemoval, setPendingRemoval] = useState<Project | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (open) setQuery("");
  }, [open]);

  const filtered = useMemo(() => {
    if (!query.trim()) return projects;
    const q = query.toLowerCase();
    return projects.filter(
      (p) =>
        p.name.toLowerCase().includes(q) || p.dir.toLowerCase().includes(q),
    );
  }, [projects, query]);

  const onConfirmRemove = async () => {
    if (!pendingRemoval) return;
    setBusy(true);
    try {
      await removeProject(pendingRemoval.id);
    } finally {
      setBusy(false);
      setPendingRemoval(null);
    }
  };

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => !o && onClose()}
        title="Switch project"
        widthClass="max-w-xl"
      >
        <div className="flex flex-col gap-2">
          <Input
            autoFocus
            placeholder="Type a project name or path…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            size="md"
          />
          <ul className="flex flex-col max-h-80 overflow-y-auto">
            {filtered.map((p) => {
              const isCurrent = currentProject?.id === p.id;
              return (
                <li key={p.id} className="group relative">
                  <button
                    className="w-full text-left pl-2 pr-10 py-2 rounded hover:bg-surface-2"
                    onClick={async () => {
                      await switchProject(p.id);
                      onClose();
                    }}
                  >
                    <div className="font-semibold flex items-center gap-2">
                      {p.name}
                      {isCurrent && (
                        <span className="text-[10px] uppercase tracking-wider text-accent">
                          current
                        </span>
                      )}
                    </div>
                    <div className="text-xs text-fg-subtle truncate">
                      {p.dir}
                    </div>
                  </button>
                  <button
                    type="button"
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 rounded opacity-0 group-hover:opacity-100 hover:bg-danger-soft hover:text-danger focus:opacity-100 transition-opacity"
                    title={`Remove "${p.name}" from the list (folder kept on disk)`}
                    onClick={(e) => {
                      e.stopPropagation();
                      setPendingRemoval(p);
                    }}
                  >
                    <TrashIcon />
                  </button>
                </li>
              );
            })}
          </ul>
          <div className="pt-2 border-t border-border-default">
            <Button
              variant="ghost"
              size="sm"
              onClick={async () => {
                const added = await pickAndAddProject();
                if (added) onClose();
              }}
            >
              + Add project…
            </Button>
          </div>
        </div>
      </Dialog>
      <ConfirmDialog
        open={pendingRemoval !== null}
        title="Remove project from list?"
        message={
          pendingRemoval
            ? `"${pendingRemoval.name}" will be removed from the recent projects list. The folder on disk is left untouched.`
            : ""
        }
        confirmLabel={busy ? "Removing…" : "Remove"}
        confirmVariant="danger"
        onConfirm={() => void onConfirmRemove()}
        onCancel={() => setPendingRemoval(null)}
      />
    </>
  );
}
