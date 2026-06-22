import { useMemo, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Select, Textarea } from "@/components/ui";
import {
  classifyContinueIntent,
  isSessionControlAction,
  type ContinueAction,
} from "@/lib/whats-next/classifyContinueIntent";
import { useUIStore } from "@/store/ui";

// QuickModeFooter is the ask_continue footer when WhatsNext Quick
// mode is on: a single free-text box replaces the action radio +
// detail form. The typed line is classified into {action, detail} by
// a local heuristic, and a dry-run banner shows the guess (with an
// action override + editable detail) so the operator confirms before
// anything runs. Low-confidence guesses surface the rationale by
// default. This keeps the operator in the loop while collapsing the
// two-field form to one line for power users.
const QUICK_ACTION_LABELS: Record<ContinueAction, string> = {
  add_ticket: "Add a ticket",
  modify_ticket: "Modify / free instruction",
  dispatch_more: "Dispatch more",
  standby: "Standby (stay reachable)",
  close: "Close the session",
};

export default function QuickModeFooter({
  busy,
  contextPrefix,
  onSubmit,
}: {
  busy: boolean;
  contextPrefix?: string;
  onSubmit: (answers: Record<string, unknown>) => void;
}) {
  const setQuickMode = useUIStore((s) => s.setWhatsNextQuickMode);
  const chatEnterSubmits = useUIStore((s) => s.chatEnterSubmits);
  const [raw, setRaw] = useState("");
  // null = follow the classifier; non-null = operator override.
  const [actionOverride, setActionOverride] = useState<ContinueAction | null>(
    null,
  );
  const [detailOverride, setDetailOverride] = useState<string | null>(null);

  const classified = useMemo(() => classifyContinueIntent(raw), [raw]);
  const action = actionOverride ?? classified.action;
  const detail = detailOverride ?? classified.detail;
  const ready = raw.trim() !== "";
  const lowConfidence = classified.confidence < 0.5;

  const noDetail = isSessionControlAction(action);
  const submit = () => {
    if (!ready || busy) return;
    // ask_continue's schema is {action, detail}; the bot's
    // derive_continue keys on action verbatim. Session-control intents
    // (standby/close) need no detail.
    onSubmit({ action, detail: noDetail ? "" : detail });
  };

  return (
    <div
      className="border-t border-border-default bg-surface-1"
      role="status"
      aria-live="polite"
    >
      <div className="mx-auto max-w-3xl px-4 py-3 space-y-2">
        {contextPrefix && contextPrefix.length > 0 && (
          <div className="text-micro text-fg-muted italic">{contextPrefix}</div>
        )}
        <Textarea
          rows={2}
          value={raw}
          placeholder="What's next? e.g. “dispatch the feature_dev tickets”, “add a ticket for the flaky sandbox boot”, “standby”."
          onChange={(e) => {
            setRaw(e.target.value);
            // Re-follow the classifier whenever the text changes; the
            // operator's prior overrides applied to stale text.
            setActionOverride(null);
            setDetailOverride(null);
          }}
          onKeyDown={(e) => {
            const submitChord = chatEnterSubmits
              ? e.key === "Enter" && !e.shiftKey
              : e.key === "Enter" && (e.metaKey || e.ctrlKey);
            if (submitChord) {
              e.preventDefault();
              submit();
            }
          }}
        />
        {ready && (
          <div
            className={`rounded border px-2 py-1.5 space-y-1.5 ${
              lowConfidence
                ? "border-warning/40 bg-warning-soft"
                : "border-border-default bg-surface-0"
            }`}
          >
            <div className="flex items-center gap-2">
              <span className="text-caption text-fg-subtle uppercase tracking-wide shrink-0">
                I'll
              </span>
              <Select
                value={action}
                onChange={(e) =>
                  setActionOverride(e.target.value as ContinueAction)
                }
                className="text-micro py-0.5"
              >
                {(Object.keys(QUICK_ACTION_LABELS) as ContinueAction[]).map(
                  (a) => (
                    <option key={a} value={a}>
                      {QUICK_ACTION_LABELS[a]}
                    </option>
                  ),
                )}
              </Select>
            </div>
            {!noDetail && (
              <input
                type="text"
                value={detail}
                onChange={(e) => setDetailOverride(e.target.value)}
                placeholder="detail (optional)"
                className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-micro text-fg-default"
              />
            )}
            {lowConfidence && (
              <div className="text-caption text-warning-fg">
                {classified.rationale} — check the action before confirming.
              </div>
            )}
          </div>
        )}
        <div className="flex items-center gap-3">
          <Button
            variant="primary"
            size="sm"
            disabled={!ready || busy}
            onClick={submit}
          >
            {busy ? "…" : "Confirm"}
          </Button>
          <button
            type="button"
            onClick={() => setQuickMode(false)}
            className="text-micro text-fg-subtle hover:text-fg-default cursor-pointer"
            title="Switch back to the structured action + detail form."
          >
            Use form instead
          </button>
        </div>
      </div>
    </div>
  );
}
