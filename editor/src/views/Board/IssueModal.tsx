import { useEffect, useState } from "react";

import type { NativeBoard, NativeIssue } from "@/api/native";

interface Props {
  board: NativeBoard;
  initial: NativeIssue | null;
  onSubmit: (input: Partial<NativeIssue>) => Promise<void> | void;
  onClose: () => void;
  onDelete?: () => void;
}

export default function IssueModal({ board, initial, onSubmit, onClose, onDelete }: Props) {
  const [title, setTitle] = useState(initial?.title ?? "");
  const [body, setBody] = useState(initial?.body ?? "");
  const [state, setState] = useState(initial?.state ?? board.states[0]?.name ?? "");
  const [labels, setLabels] = useState((initial?.labels ?? []).join(", "));
  const [priority, setPriority] = useState(initial?.priority ?? 0);
  const [assignee, setAssignee] = useState(initial?.assignee ?? "");
  const [fields, setFields] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    for (const f of board.fields ?? []) {
      const v = initial?.fields?.[f.name];
      out[f.name] = v == null ? "" : String(v);
    }
    return out;
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      // Don't swallow ESC that's meant to dismiss a textarea, an
      // input, or a native <select>'s dropdown — closing the modal
      // there would destroy unsaved edits.
      const t = e.target as Element | null;
      if (
        t instanceof HTMLTextAreaElement ||
        t instanceof HTMLInputElement ||
        t instanceof HTMLSelectElement
      ) {
        return;
      }
      onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const out: Partial<NativeIssue> = {
      title: title.trim(),
      body: body.trim(),
      state,
      labels: labels
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
      priority,
      assignee: assignee.trim() || undefined,
    };
    const typedFields = coerceFields(board, fields);
    if (Object.keys(typedFields).length > 0) {
      out.fields = typedFields;
    }
    await onSubmit(out);
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 bg-black/40 flex items-center justify-center z-50"
      onClick={onClose}
    >
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={submit}
        className="bg-surface-1 border border-border-default rounded shadow-2xl w-[36rem] max-w-[95vw] max-h-[90vh] overflow-auto"
      >
        <header className="px-4 py-2.5 border-b border-border-default flex items-center justify-between">
          <h2 className="text-sm font-semibold">
            {initial ? "Edit issue" : "New issue"}
          </h2>
          <button
            type="button"
            className="text-fg-muted hover:text-fg-default text-xl leading-none"
            onClick={onClose}
            aria-label="Close"
          >
            ×
          </button>
        </header>

        <div className="p-4 space-y-3">
          <Field label="Title">
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className={inputClasses}
              required
            />
          </Field>

          <Field label="Body">
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              className={inputClasses + " h-24 font-mono text-xs"}
            />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label="State">
              <select
                value={state}
                onChange={(e) => setState(e.target.value)}
                className={inputClasses}
                disabled={!!initial /* state changes go through transition for the audit log */}
              >
                {board.states.map((s) => (
                  <option key={s.name} value={s.name}>
                    {s.display ?? s.name}
                  </option>
                ))}
              </select>
              {initial && (
                <p className="text-xs text-fg-muted mt-1">
                  Drag the card to move between states.
                </p>
              )}
            </Field>
            <Field label="Priority">
              <input
                type="number"
                value={priority}
                onChange={(e) => setPriority(parseInt(e.target.value || "0", 10))}
                className={inputClasses}
              />
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Labels (comma-separated)">
              <input
                value={labels}
                onChange={(e) => setLabels(e.target.value)}
                className={inputClasses}
                placeholder="urgent, infra"
              />
            </Field>
            <Field label="Assignee">
              <input
                value={assignee}
                onChange={(e) => setAssignee(e.target.value)}
                className={inputClasses}
              />
            </Field>
          </div>

          {(board.fields ?? []).map((f) => (
            <Field key={f.name} label={(f.display ?? f.name) + ` (${f.type})`}>
              {f.type === "enum" ? (
                <select
                  value={fields[f.name] ?? ""}
                  onChange={(e) =>
                    setFields({ ...fields, [f.name]: e.target.value })
                  }
                  className={inputClasses}
                >
                  <option value="">(unset)</option>
                  {(f.enum_values ?? []).map((v) => (
                    <option key={v} value={v}>
                      {v}
                    </option>
                  ))}
                </select>
              ) : f.type === "bool" ? (
                <input
                  type="checkbox"
                  checked={fields[f.name] === "true"}
                  onChange={(e) =>
                    setFields({ ...fields, [f.name]: e.target.checked ? "true" : "false" })
                  }
                />
              ) : (
                <input
                  type={f.type === "number" ? "number" : f.type === "date" ? "datetime-local" : "text"}
                  value={fields[f.name] ?? ""}
                  onChange={(e) =>
                    setFields({ ...fields, [f.name]: e.target.value })
                  }
                  className={inputClasses}
                  required={f.required}
                />
              )}
            </Field>
          ))}
        </div>

        <footer className="px-4 py-2.5 border-t border-border-default flex items-center justify-between bg-surface-0">
          <div>
            {onDelete && (
              <button
                type="button"
                onClick={onDelete}
                className="text-xs text-red-400 hover:underline"
              >
                Delete
              </button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="text-xs px-3 py-1.5 rounded border border-border-default hover:bg-surface-2"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="text-xs px-3 py-1.5 rounded bg-accent text-on-accent hover:opacity-90"
            >
              {initial ? "Save" : "Create"}
            </button>
          </div>
        </footer>
      </form>
    </div>
  );
}

const inputClasses =
  "w-full bg-surface-0 border border-border-default rounded px-2 py-1.5 text-sm text-fg-default focus:outline-none focus:border-accent/60";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-xs text-fg-muted block mb-1">{label}</span>
      {children}
    </label>
  );
}

// coerceFields converts the modal's string-keyed state map into the
// typed shape the API expects (numbers, bools, etc.). Date fields are
// expected as RFC3339 strings — the datetime-local input emits
// "YYYY-MM-DDThh:mm" which is acceptable since the server stores it
// verbatim and only validates parseability.
function coerceFields(board: NativeBoard, raw: Record<string, string>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of board.fields ?? []) {
    const v = raw[f.name];
    if (v == null || v === "") continue;
    switch (f.type) {
      case "number": {
        const n = Number(v);
        if (Number.isFinite(n)) out[f.name] = n;
        break;
      }
      case "bool":
        out[f.name] = v === "true";
        break;
      case "date":
        // Convert "YYYY-MM-DDThh:mm" → RFC3339 with Z.
        out[f.name] = v.includes("Z") || v.includes("+") ? v : v + ":00Z";
        break;
      default:
        out[f.name] = v;
    }
  }
  return out;
}
