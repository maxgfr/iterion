import { errorMessage } from "@/lib/errorHints";
import { useEffect, useMemo, useState } from "react";

import { resumeRun } from "@/api/runs";
import { Button, Textarea } from "@/components/ui";
import { useDocumentStore } from "@/store/document";

interface Props {
  runId: string;
  // Map of field name → question text. Mirror of
  // store.Checkpoint.InteractionQuestions / human_input_requested
  // event payload.
  questions: Record<string, unknown>;
  // Optional one-line description that the agent surfaced on pause
  // (e.g. "Awaiting your approval to merge"). Comes from event data.
  message?: string;
  onSubmitted?: () => void;
}

// A permission-gate `ask` pause carries a structured marker under the
// reserved `_permission` key (mirrors pkg/backend/permission.Marker). When
// present we render a one-click approval card instead of free-text fields.
const PERMISSION_MARKER_KEY = "_permission";
const ASK_USER_KEY = "ask_user_response";

interface PermissionMarker {
  tool?: string;
  input?: Record<string, unknown>;
  rule?: string;
}

function permissionMarker(questions: Record<string, unknown>): PermissionMarker | null {
  const m = questions?.[PERMISSION_MARKER_KEY];
  if (m && typeof m === "object" && !Array.isArray(m)) return m as PermissionMarker;
  return null;
}

// The most identifying argument of a tool call, for compact display.
function briefInput(input?: Record<string, unknown>): string {
  if (!input) return "";
  for (const k of ["command", "file_path", "path", "url", "pattern", "query"]) {
    const v = input[k];
    if (typeof v === "string" && v) return v;
  }
  try {
    return JSON.stringify(input);
  } catch {
    return "";
  }
}

export default function PauseForm({ runId, questions, message, onSubmitted }: Props) {
  const marker = useMemo(() => permissionMarker(questions ?? {}), [questions]);
  const fieldNames = useMemo(() => Object.keys(questions ?? {}), [questions]);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(fieldNames.map((k) => [k, ""])),
  );
  // Reset draft answers when the question set changes (e.g. a second
  // pause on the same run with different fields, or a navigation
  // between two paused runs without unmount). The lazy initialiser
  // above runs once; without this, new field names show old values
  // and old field names leak into the submit payload.
  const fieldKey = fieldNames.join("\x00");
  useEffect(() => {
    setValues(Object.fromEntries(fieldNames.map((k) => [k, ""])));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId, fieldKey]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const currentSource = useDocumentStore((s) => s.currentSource);

  const onChange = (name: string, next: string) => {
    setValues((prev) => ({ ...prev, [name]: next }));
  };

  const onSubmit = async () => {
    setBusy(true);
    setError(null);
    try {
      // The runtime accepts a generic answers map; values are passed
      // through to the resumed node's inputs. Strings are the safest
      // common type for an ad-hoc pause UI.
      await resumeRun(runId, { answers: values, source: currentSource ?? undefined });
      onSubmitted?.();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  // Permission approval: submit the operator's decision under the
  // ask_user key; the runtime turns "allow"/"allow always" into a grant
  // rule and "deny" into a refusal.
  const decide = async (decision: "allow" | "allow always" | "deny") => {
    setBusy(true);
    setError(null);
    try {
      await resumeRun(runId, {
        answers: { [ASK_USER_KEY]: decision },
        source: currentSource ?? undefined,
      });
      onSubmitted?.();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  if (marker) {
    const prompt = String(questions[ASK_USER_KEY] ?? "");
    const arg = briefInput(marker.input);
    return (
      <div className="space-y-3">
        {message && <p className="text-fg-muted text-micro">{message}</p>}
        <div className="rounded-md border border-warning/40 bg-warning-soft p-3 space-y-2">
          <div className="text-micro font-medium text-fg-default">
            🔐 Approval required: <code>{marker.tool}</code>
          </div>
          {arg && (
            <pre className="text-caption text-fg-subtle whitespace-pre-wrap break-all max-h-32 overflow-auto m-0">
              {arg}
            </pre>
          )}
          {!arg && prompt && (
            <div className="text-caption text-fg-subtle whitespace-pre-wrap">{prompt}</div>
          )}
        </div>
        {error && (
          <p className="text-danger-fg text-micro" role="alert">
            {error}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button variant="primary" size="sm" loading={busy} onClick={() => void decide("allow")}>
            Allow once
          </Button>
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => void decide("allow always")}>
            Allow always
          </Button>
          <Button variant="danger" size="sm" disabled={busy} onClick={() => void decide("deny")}>
            Deny
          </Button>
        </div>
      </div>
    );
  }

  if (fieldNames.length === 0) {
    return (
      <div className="space-y-3">
        {message && (
          <p className="text-fg-muted text-micro">{message}</p>
        )}
        <p className="text-fg-subtle text-micro">
          This run paused without specific questions. Resume to continue.
        </p>
        <Button
          variant="primary"
          size="sm"
          onClick={() => void onSubmit()}
          loading={busy}
        >
          Resume
        </Button>
        <div role="status" aria-live="polite">
          {error && (
            <p className="text-danger-fg text-micro" role="alert">
              {error}
            </p>
          )}
        </div>
      </div>
    );
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void onSubmit();
      }}
      className="space-y-3"
    >
      {message && <p className="text-fg-muted text-micro">{message}</p>}
      {fieldNames.map((name) => {
        const prompt = String(questions[name] ?? "");
        return (
          <label key={name} className="block space-y-1">
            <div className="text-micro font-medium text-fg-default">{name}</div>
            {prompt && (
              <div className="text-caption text-fg-subtle whitespace-pre-wrap">
                {prompt}
              </div>
            )}
            <Textarea
              value={values[name] ?? ""}
              onChange={(e) => onChange(name, e.target.value)}
              rows={prompt.length > 80 ? 4 : 2}
              spellCheck={false}
              className="text-micro"
            />
          </label>
        );
      })}
      {error && (
        <p className="text-danger-fg text-micro" role="alert">
          {error}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" variant="primary" size="sm" loading={busy}>
          Submit &amp; Resume
        </Button>
      </div>
    </form>
  );
}
