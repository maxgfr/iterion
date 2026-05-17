import { useEffect, useRef } from "react";

import type { FirstClassBot } from "@/lib/pilote/firstClassBots";
import type { PiloteMessage } from "@/lib/pilote/messages";
import type { FormAnswer } from "@/lib/pilote/questionForm";

import HumanChatTurn from "./HumanChatTurn";
import IssuesSummaryCard from "./IssuesSummaryCard";
import NodeBanner from "./NodeBanner";
import RoadmapCard from "./RoadmapCard";
import SurveyCard from "./SurveyCard";

interface Props {
  messages: PiloteMessage[];
  // The active bot — used to look up nodeMap form specs for human
  // turns and to enrich rendering with bot-specific affordances.
  bot?: FirstClassBot;
  // Called when the user submits a reply to a pending human-question
  // message. The `messageId` is the id of the human-question message
  // being answered, so callers can route the submit back to the
  // matching interaction. `outcome.formAnswer` is populated when the
  // node has a rich form spec; otherwise the parent falls back to
  // text + approved.
  onHumanSubmit?: (
    messageId: string,
    outcome: {
      text: string;
      approved?: boolean;
      formAnswer?: FormAnswer;
    },
  ) => void;
  // True while a submit is in-flight; disables inputs on the pending
  // human-question turn.
  busyMessageId?: string | null;
}

export default function ChatTranscript({
  messages,
  bot,
  onHumanSubmit,
  busyMessageId = null,
}: Props) {
  const endRef = useRef<HTMLDivElement | null>(null);
  // Auto-scroll to the latest message on every change. Cheap given
  // the transcript stays under ~50 messages in practice (a whats-next
  // session is bounded by approval_loop(10)).
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages.length]);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">
      {messages.map((m) => (
        <MessageRow
          key={m.id}
          message={m}
          bot={bot}
          onHumanSubmit={onHumanSubmit}
          busy={m.kind === "human-question" && busyMessageId === m.id}
        />
      ))}
      {messages.length === 0 && (
        <p className="text-[12px] text-fg-subtle italic">
          The session will start as soon as iterion finishes the first survey.
        </p>
      )}
      <div ref={endRef} />
    </div>
  );
}

function MessageRow({
  message,
  bot,
  onHumanSubmit,
  busy,
}: {
  message: PiloteMessage;
  bot?: FirstClassBot;
  onHumanSubmit?: Props["onHumanSubmit"];
  busy: boolean;
}) {
  switch (message.kind) {
    case "banner":
      return <NodeBanner message={message} />;
    case "human-question": {
      const form = bot?.nodeMap[message.nodeId]?.form;
      return (
        <HumanChatTurn
          message={message}
          form={form}
          onSubmit={
            onHumanSubmit
              ? (outcome) => onHumanSubmit(message.id, outcome)
              : undefined
          }
          busy={busy}
        />
      );
    }
    case "roadmap-card":
      return <RoadmapCard message={message} />;
    case "issues-summary":
      return <IssuesSummaryCard message={message} />;
    case "survey-card":
      return <SurveyCard message={message} />;
    case "session-closed":
      return <SessionClosedRow message={message} />;
  }
}

function SessionClosedRow({
  message,
}: {
  message: Extract<PiloteMessage, { kind: "session-closed" }>;
}) {
  const label =
    message.reason === "finished"
      ? "Session closed — plan handed off."
      : message.reason === "failed"
        ? "Session failed."
        : "Session cancelled.";
  const cls =
    message.reason === "finished"
      ? "text-fg-muted"
      : message.reason === "failed"
        ? "text-danger-fg"
        : "text-warning-fg";
  return (
    <div
      className={`text-[11px] text-center italic border-t border-border-subtle pt-3 ${cls}`}
    >
      {label}
    </div>
  );
}
