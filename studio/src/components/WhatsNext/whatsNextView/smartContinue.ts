import type { WhatsNextMessage } from "@/lib/whats-next/messages";

// previousContinueAction returns the `action` the operator picked on
// the most recent ANSWERED ask_continue turn, or "" when none exists
// yet (first loop iteration). Reads the structured answers persisted
// on the turn's outcome (runChat stores the full answers map there).
export function previousContinueAction(
  upstream: ReadonlyArray<WhatsNextMessage>,
): string {
  for (let i = upstream.length - 1; i >= 0; i--) {
    const m = upstream[i];
    if (
      m &&
      m.kind === "human-question" &&
      m.nodeId === "ask_continue" &&
      m.status === "answered"
    ) {
      const action = m.outcome?.action;
      return typeof action === "string" ? action : "";
    }
  }
  return "";
}

// smartContinueDefault maps the previous loop's action to the
// next-likely action to pre-select. Returns undefined when there's
// no useful (or safe) default — the caller leaves the radio
// unselected so the operator picks deliberately.
//   add_ticket    → add another (operators batch ticket creation)
//   modify_ticket → add a ticket (typical triage rhythm)
//   dispatch_*    → undefined (no default)
// `done` is never produced as a default: ending the session must be
// an explicit pick, never a one-Enter accident. After a dispatch the
// operator's next move is genuinely ambiguous (end / dispatch more /
// add follow-up), so we don't guess — pre-selecting `done` there
// would risk an accidental session-end.
export function smartContinueDefault(
  previousAction: string,
): string | undefined {
  switch (previousAction) {
    case "add_ticket":
    case "modify_ticket":
      return "add_ticket";
    default:
      return undefined;
  }
}
