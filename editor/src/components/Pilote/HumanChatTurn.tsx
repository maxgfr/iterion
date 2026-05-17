import { useState } from "react";

import type { HumanQuestionMessage } from "@/lib/pilote/messages";
import { Button, Textarea } from "@/components/ui";

interface Props {
  message: HumanQuestionMessage;
  // Called by the user. Wired in Étape 3 — for Étape 1 the parent
  // passes a no-op (or local-state updater for mock progression).
  onSubmit?: (outcome: { text: string; approved?: boolean }) => void;
  busy?: boolean;
}

export default function HumanChatTurn({ message, onSubmit, busy = false }: Props) {
  const [draft, setDraft] = useState("");
  const [reviseOpen, setReviseOpen] = useState(false);

  if (message.status === "answered") {
    return <AnsweredTurn message={message} />;
  }

  const hasActions = (message.actions?.length ?? 0) > 0;
  const isFreeText = !hasActions;

  const submit = (approved?: boolean) => {
    if (!onSubmit || busy) return;
    onSubmit({ text: draft, approved });
    setDraft("");
    setReviseOpen(false);
  };

  return (
    <div className="space-y-2">
      <AssistantBubble text={message.prompt} />

      {isFreeText && (
        <div className="flex items-stretch gap-2 ml-6">
          <Textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Type your answer…"
            rows={Math.max(2, Math.min(8, Math.ceil(draft.length / 60) + 1))}
            disabled={busy}
            className="flex-1"
          />
          <Button
            variant="primary"
            size="sm"
            disabled={busy || draft.trim() === ""}
            onClick={() => submit()}
            className="self-end"
          >
            {busy ? "…" : "Send"}
          </Button>
        </div>
      )}

      {hasActions && (
        <div className="ml-6 space-y-2">
          <div className="flex items-center gap-2">
            <Button
              variant="primary"
              size="sm"
              disabled={busy}
              onClick={() => submit(true)}
            >
              {busy ? "…" : "Approve"}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              disabled={busy}
              onClick={() => setReviseOpen((v) => !v)}
            >
              Request revision
            </Button>
          </div>

          {reviseOpen && (
            <div className="flex items-stretch gap-2">
              <Textarea
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="What should be revised?"
                rows={3}
                disabled={busy}
                className="flex-1"
              />
              <Button
                variant="primary"
                size="sm"
                disabled={busy || draft.trim() === ""}
                onClick={() => submit(false)}
                className="self-end"
              >
                {busy ? "…" : "Send"}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function AssistantBubble({ text }: { text: string }) {
  return (
    <div className="flex items-start gap-2">
      <span
        className="mt-1 w-5 h-5 rounded-full bg-accent-soft text-accent-fg text-[10px] font-bold flex items-center justify-center shrink-0"
        aria-hidden="true"
      >
        AI
      </span>
      <div className="flex-1 rounded-lg bg-surface-2 border border-border-subtle px-3 py-2 text-[13px] text-fg-default">
        {text}
      </div>
    </div>
  );
}

function AnsweredTurn({ message }: { message: HumanQuestionMessage }) {
  return (
    <div className="space-y-2">
      <AssistantBubble text={message.prompt} />
      <div className="flex items-start gap-2 ml-6">
        <span
          className="mt-1 w-5 h-5 rounded-full bg-surface-3 text-fg-default text-[10px] font-bold flex items-center justify-center shrink-0"
          aria-hidden="true"
        >
          You
        </span>
        <div className="flex-1 rounded-lg bg-surface-1 border border-border-subtle px-3 py-2 text-[13px] text-fg-default whitespace-pre-wrap">
          {message.userReply || (
            <span className="italic text-fg-subtle">
              {message.outcome && "approved" in message.outcome
                ? message.outcome.approved
                  ? "approved"
                  : "requested revision"
                : "(empty reply)"}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
