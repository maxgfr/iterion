import { useCallback, useEffect, useMemo, useState } from "react";

import NavLinks from "@/components/shared/NavLinks";
import ProjectLabel from "@/components/shared/ProjectLabel";
import {
  createIssue,
  deleteIssue,
  getBoard,
  listIssues,
  patchIssue,
  transitionIssue,
  type NativeBoard,
  type NativeIssue,
} from "@/api/native";
import IssueModal from "./IssueModal";

export default function BoardView() {
  const [board, setBoard] = useState<NativeBoard | null>(null);
  const [issues, setIssues] = useState<NativeIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<NativeIssue | null>(null);
  const [creating, setCreating] = useState(false);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [b, i] = await Promise.all([getBoard(), listIssues()]);
      setBoard(b);
      setIssues(i ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Group issues by state for column rendering. Issues whose state
  // does not appear on the board land in an "unmapped" bucket so they
  // are not silently lost when the operator renames a state.
  const byState = useMemo(() => {
    const m = new Map<string, NativeIssue[]>();
    if (!board) return m;
    for (const s of board.states) m.set(s.name, []);
    m.set("__unmapped__", []);
    for (const iss of issues) {
      const bucket = m.has(iss.state) ? iss.state : "__unmapped__";
      m.get(bucket)!.push(iss);
    }
    for (const list of m.values()) {
      list.sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0));
    }
    return m;
  }, [board, issues]);

  const onDrop = useCallback(
    async (issueID: string, toState: string) => {
      const before = issues;
      // optimistic update
      setIssues((cur) => cur.map((i) => (i.id === issueID ? { ...i, state: toState } : i)));
      try {
        await transitionIssue(issueID, toState);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        setIssues(before);
      }
    },
    [issues],
  );

  const onCreate = useCallback(
    async (input: Partial<NativeIssue>) => {
      try {
        await createIssue({
          title: input.title ?? "",
          body: input.body,
          state: input.state,
          labels: input.labels,
          priority: input.priority,
          assignee: input.assignee,
          fields: input.fields,
        });
        setCreating(false);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [refresh],
  );

  const onSave = useCallback(
    async (input: Partial<NativeIssue>) => {
      if (!editing) return;
      try {
        await patchIssue(editing.id, {
          title: input.title,
          body: input.body,
          labels: input.labels,
          priority: input.priority,
          assignee: input.assignee,
          fields: input.fields,
        });
        setEditing(null);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [editing, refresh],
  );

  const onDelete = useCallback(
    async (id: string) => {
      if (!confirm("Delete this issue?")) return;
      try {
        await deleteIssue(id);
        setEditing(null);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [refresh],
  );

  if (loading) {
    return <div className="p-8 text-fg-muted">Loading kanban…</div>;
  }
  if (!board) {
    return (
      <div className="p-8 text-fg-muted">
        Native tracker not available.{" "}
        <code className="text-xs">iterion editor --dir &lt;project&gt;</code> creates one on first launch.
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col overflow-hidden bg-surface-0 text-fg-default">
      <header className="border-b border-border-default px-4 py-2.5 flex items-center gap-3 bg-surface-1">
        <span className="text-sm font-bold tracking-wide">ITERION</span>
        <NavLinks active="board" />
        <ProjectLabel variant="header" />
        <div className="ml-auto flex items-center gap-2">
          <button
            className="text-xs px-2 py-1 rounded border border-border-default hover:bg-surface-2"
            onClick={() => void refresh()}
          >
            Refresh
          </button>
          <button
            className="text-xs px-2 py-1 rounded bg-accent text-on-accent hover:opacity-90"
            onClick={() => setCreating(true)}
          >
            + New issue
          </button>
        </div>
      </header>

      {error && (
        <div className="bg-red-500/10 border-b border-red-500/40 px-4 py-2 text-xs text-red-200">
          {error}
        </div>
      )}

      <main className="flex-1 overflow-auto p-3">
        <div className="flex gap-3 min-w-fit">
          {board.states.map((s) => (
            <Column
              key={s.name}
              name={s.name}
              display={s.display ?? s.name}
              terminal={!!s.terminal}
              eligible={!!s.eligible}
              issues={byState.get(s.name) ?? []}
              onDrop={onDrop}
              onClickCard={(iss) => setEditing(iss)}
            />
          ))}
          {(byState.get("__unmapped__")?.length ?? 0) > 0 && (
            <Column
              name="__unmapped__"
              display="Unmapped"
              terminal={false}
              eligible={false}
              issues={byState.get("__unmapped__") ?? []}
              onDrop={onDrop}
              onClickCard={(iss) => setEditing(iss)}
            />
          )}
        </div>
      </main>

      {creating && (
        <IssueModal
          board={board}
          initial={null}
          onSubmit={onCreate}
          onClose={() => setCreating(false)}
        />
      )}
      {editing && (
        <IssueModal
          board={board}
          initial={editing}
          onSubmit={onSave}
          onClose={() => setEditing(null)}
          onDelete={() => void onDelete(editing.id)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Column + Card
// ---------------------------------------------------------------------------

interface ColumnProps {
  name: string;
  display: string;
  terminal: boolean;
  eligible: boolean;
  issues: NativeIssue[];
  onDrop: (issueID: string, toState: string) => void;
  onClickCard: (iss: NativeIssue) => void;
}

function Column({ name, display, terminal, eligible, issues, onDrop, onClickCard }: ColumnProps) {
  const [dragOver, setDragOver] = useState(false);
  return (
    <div
      className={`w-72 shrink-0 rounded border ${
        dragOver ? "border-accent/60 bg-accent-soft/30" : "border-border-default bg-surface-1"
      } flex flex-col`}
      onDragOver={(e) => {
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragOver(false);
        const id = e.dataTransfer.getData("text/plain");
        if (id && name !== "__unmapped__") onDrop(id, name);
      }}
    >
      <div className="px-3 py-2 border-b border-border-default flex items-center justify-between text-xs">
        <span className="font-semibold uppercase tracking-wide text-fg-default">{display}</span>
        <span className="text-fg-muted">
          {issues.length}
          {eligible && <span className="ml-1 text-emerald-400">●</span>}
          {terminal && <span className="ml-1 text-fg-muted">✓</span>}
        </span>
      </div>
      <div className="p-2 flex-1 flex flex-col gap-2 overflow-auto">
        {issues.map((iss) => (
          <IssueCard key={iss.id} iss={iss} onClick={() => onClickCard(iss)} />
        ))}
        {issues.length === 0 && (
          <div className="text-xs text-fg-muted text-center py-4">drop here</div>
        )}
      </div>
    </div>
  );
}

function IssueCard({ iss, onClick }: { iss: NativeIssue; onClick: () => void }) {
  return (
    <div
      role="button"
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData("text/plain", iss.id);
        e.dataTransfer.effectAllowed = "move";
      }}
      onClick={onClick}
      className="bg-surface-0 border border-border-default rounded p-2 text-sm cursor-grab hover:border-accent/40 active:cursor-grabbing"
    >
      <div className="flex items-start gap-2">
        <span className="text-fg-default flex-1">{iss.title}</span>
        {iss.priority && iss.priority > 0 ? (
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-300">
            P{iss.priority}
          </span>
        ) : null}
      </div>
      {(iss.labels?.length ?? 0) > 0 && (
        <div className="mt-1 flex flex-wrap gap-1">
          {iss.labels!.map((l) => (
            <span
              key={l}
              className="text-[10px] px-1.5 py-0.5 rounded bg-surface-2 text-fg-muted"
            >
              {l}
            </span>
          ))}
        </div>
      )}
      <div className="mt-1 flex items-center gap-2 text-[10px] text-fg-muted">
        <code className="opacity-70">{shortID(iss.id)}</code>
        {iss.assignee && <span>@{iss.assignee}</span>}
        {iss.claim && <span className="text-amber-300">claimed</span>}
      </div>
    </div>
  );
}

function shortID(id: string) {
  const bare = id.replace(/^native:/, "").replace(/^github:[^#]+#/, "#");
  return bare.length > 10 ? bare.slice(0, 10) : bare;
}
