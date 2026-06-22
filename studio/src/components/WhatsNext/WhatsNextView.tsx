import { useCallback, useEffect, useRef } from "react";

import {
  DEFAULT_WHATS_NEXT_BOT_ID,
  getFirstClassBot,
} from "@/lib/whats-next/firstClassBots";
import { useWhatsNextSession } from "@/lib/whats-next/useWhatsNextSession";
import type { FormAnswer } from "@/lib/whats-next/questionForm";

import { queueMessage } from "@/api/queueMessages";
import AgentChatbox from "@/components/shared/AgentChatbox";
import { useUIStore } from "@/store/ui";
import ChatTranscript from "./ChatTranscript";
import PreFlightPanel from "./PreFlightPanel";
import SessionLauncher from "./SessionLauncher";
import WatchPanel from "./WatchPanel";
import PendingTurnFooter from "./whatsNextView/PendingTurnFooter";
import QuickModeFooter from "./whatsNextView/QuickModeFooter";
import ResumeFooter from "./whatsNextView/ResumeFooter";
import SessionHeader from "./whatsNextView/SessionHeader";
import { composerPlaceholder } from "./whatsNextView/composerPlaceholder";
import { contextPrefixFor, resolveDynamicForm } from "./whatsNextView/forms";

// Re-export the pure helpers covered by smartContinue.test.ts /
// humanStatus.test.ts so the test imports `from "./WhatsNextView"`
// continue to resolve unchanged.
export {
  previousContinueAction,
  smartContinueDefault,
} from "./whatsNextView/smartContinue";
export { humanStatus } from "./whatsNextView/humanStatus";

// WhatsNextView is the /whats-next route. It owns one whats-next session at a
// time via the useWhatsNextSession hook: the launcher creates the run,
// the transcript reads from the run store, and human turns are
// submitted via the hook's submitHumanAnswer.

export default function WhatsNextView() {
  const bot = getFirstClassBot(DEFAULT_WHATS_NEXT_BOT_ID);
  const quickMode = useUIStore((s) => s.whatsNextQuickMode);
  // Hooks must be called unconditionally — pass a dummy bot if the
  // lookup miss happens (in practice it can't since DEFAULT_WHATS_NEXT_BOT_ID
  // is a const key, but the early-return branch needs valid hook order).
  const session = useWhatsNextSession(
    bot ?? {
      id: "",
      label: "",
      description: "",
      workflowPath: "",
      launcherVars: [],
      nodeMap: {},
    },
  );

  // Stashed launcher form answer awaiting auto-submission into the
  // first matching pending human turn. Lives in a ref so the
  // auto-submit effect can read + clear it without re-rendering when
  // unrelated state changes. Operators who refresh the page mid-run
  // before the bot reaches the target turn lose the stash and answer
  // the form once it appears in the chat — acceptable degradation.
  const pendingLauncherAnswer = useRef<FormAnswer | null>(null);

  // submit is shaped to match HumanChatTurn's contract:
  //   outcome = { text, approved?, formAnswer? }
  //
  // When a node declares a rich `form:` in nodeMap, the FormAnswer's
  // question ids ARE the answer keys, so we forward verbatim.
  // Otherwise we look up textField/approvedField to build the
  // answers object from the legacy text + actions UI:
  //   ask_priorities → { context: text }
  //   human_review   → { feedback: text, approved: bool }
  const onHumanSubmit = useCallback(
    (
      messageId: string,
      outcome: {
        text: string;
        approved?: boolean;
        formAnswer?: Record<string, string | string[]>;
      },
    ) => {
      const m = bot
        ? session.messages.find((x) => x.id === messageId)
        : undefined;
      const entry = m && m.kind === "human-question" && bot
        ? bot.nodeMap[m.nodeId]
        : undefined;
      if (!bot || !m || m.kind !== "human-question" || !entry) return;
      // When the chat turn supplies BOTH a formAnswer (from the
      // checkbox column, e.g. human_review's selected_titles) AND
      // approved/text (from the Approve / Request-revision buttons),
      // merge the two: the form covers the structured fields the
      // bot declared in its output schema, the action covers the
      // verdict + feedback. Either alone (form-only turn, or pure
      // actions turn) keeps the existing behaviour.
      const answers: Record<string, unknown> = outcome.formAnswer
        ? { ...outcome.formAnswer }
        : {};
      if (entry.textField && outcome.text !== "") {
        answers[entry.textField] = outcome.text;
      }
      if (entry.approvedField && outcome.approved !== undefined) {
        answers[entry.approvedField] = outcome.approved;
      }
      void session.submitHumanAnswer(messageId, answers);
    },
    [bot, session],
  );

  // onComposerSend backs the always-on composer (the footer's last
  // branch). It renders only when there's no pending human turn, so
  // the run is either still working or already closed:
  //   - closed (no run / finished / failed / cancelled) → re-seed a
  //     fresh session, delivering this message as the ask_priorities
  //     answer (auto-submitted by the launcher-stash effect below).
  //     Nexie's workspace memory reloads on the new run, so it picks
  //     up where the closed session left off — continuity without a
  //     resumable engine (a finished run can't be resumed).
  //   - working (running/queued) → inject the message into the live
  //     agent loop's inbox via queueMessage.
  const onComposerSend = useCallback(
    async (text: string, opts: { skills: string[] }) => {
      const trimmed = text.trim();
      if (trimmed === "") return;
      const status = session.runStatus;
      const closed =
        !session.runId ||
        status === "finished" ||
        status === "failed" ||
        status === "cancelled";
      if (closed) {
        pendingLauncherAnswer.current = { context: trimmed };
        await session.launch(session.lastVars ?? {});
        return;
      }
      // Not closed ⇒ runId is truthy (it's part of the `closed`
      // disjunction above), so the run is live: inject into its inbox.
      await queueMessage(session.runId!, trimmed, { skills: opts.skills });
    },
    [session],
  );

  // pendingHumanQuestion + the launcher-stash auto-submit effect are
  // declared before the early return below so the effect runs on every
  // render (rules-of-hooks); it no-ops until a bot is present.
  const pendingHumanQuestion = session.messages.find(
    (m): m is Extract<typeof m, { kind: "human-question" }> =>
      m.kind === "human-question" && m.status === "pending",
  );
  // Auto-submit the stashed launcher form answer into the first pending
  // human-question whose node id matches launcherFormTarget. The operator
  // picked their priority before the bot ran; once explore finishes and
  // ask_priorities surfaces, we resolve it silently rather than re-asking.
  useEffect(() => {
    if (!bot || !bot.launcherFormTarget) return;
    if (!pendingHumanQuestion) return;
    if (pendingHumanQuestion.nodeId !== bot.launcherFormTarget) return;
    const stash = pendingLauncherAnswer.current;
    if (!stash) return;
    pendingLauncherAnswer.current = null;
    void session.submitHumanAnswer(pendingHumanQuestion.id, stash);
  }, [pendingHumanQuestion, bot?.launcherFormTarget, session]);

  if (!bot) {
    return (
      <div className="h-full grid place-items-center text-fg-muted">
        No first-class bot registered.
      </div>
    );
  }

  const inSession = session.status !== "idle";

  // When the engine is waiting on a human turn, render that turn at
  // the bottom (in a fixed-footer wrapper) instead of the generic
  // AgentChatbox. Avoids the inline + footer double-render by passing
  // excludeMessageId to ChatTranscript. (pendingHumanQuestion is declared
  // above with the launcher-stash effect to keep that hook unconditional.)
  const pendingForm = pendingHumanQuestion
    ? resolveDynamicForm(pendingHumanQuestion, session.messages, bot.nodeMap)
    : undefined;

  return (
    <div className="h-full flex flex-col overflow-hidden">
        {!inSession ? (
          <SessionLauncher
            bot={bot}
            onLaunch={({ vars, formAnswer }) => {
              if (formAnswer) pendingLauncherAnswer.current = formAnswer;
              void session.launch(vars);
            }}
            busy={session.status === "launching"}
            errorMessage={session.errorMessage}
          />
        ) : (
          <div className="flex-1 flex flex-col max-w-3xl w-full mx-auto overflow-hidden">
            <SessionHeader bot={bot} session={session} />
            <WatchPanel runId={session.runId} />
            {session.messages.length === 0 ? (
              <div className="flex-1 overflow-y-auto">
                <PreFlightPanel
                  runId={session.runId}
                  runStatus={session.runStatus}
                />
              </div>
            ) : (
              <ChatTranscript
                messages={session.messages}
                bot={bot}
                onHumanSubmit={onHumanSubmit}
                busyMessageId={session.busyMessageId}
                excludeMessageId={pendingHumanQuestion?.id}
              />
            )}
            {session.errorMessage && (
              <div className="border-t border-danger/40 bg-danger-soft px-4 py-2 text-body text-danger-fg">
                {session.errorMessage}
              </div>
            )}
            {session.runId &&
            (session.runStatus === "failed_resumable" ||
              session.runStatus === "cancelled") ? (
              // Terminal-but-resumable wins over PendingTurnFooter:
              // the form on a dead run is misleading — submitting goes
              // through resumeRun which would force-resume the engine
              // anyway, but the operator's mental model "the form is
              // active" is wrong. Surface Resume explicitly first; the
              // form's pending question gets re-shown by the new
              // engine instance after Resume kicks the run forward.
              <ResumeFooter
                runStatus={session.runStatus}
                busy={session.status === "submitting"}
                onResume={() => void session.resume()}
              />
            ) : pendingHumanQuestion &&
              quickMode &&
              pendingHumanQuestion.nodeId === "ask_continue" ? (
              <QuickModeFooter
                busy={session.busyMessageId === pendingHumanQuestion.id}
                contextPrefix={contextPrefixFor(
                  pendingHumanQuestion,
                  session.messages,
                )}
                onSubmit={(answers) =>
                  void session.submitHumanAnswer(pendingHumanQuestion.id, answers)
                }
              />
            ) : pendingHumanQuestion ? (
              <PendingTurnFooter
                message={pendingHumanQuestion}
                form={pendingForm}
                busy={session.busyMessageId === pendingHumanQuestion.id}
                contextPrefix={contextPrefixFor(
                  pendingHumanQuestion,
                  session.messages,
                )}
                onSubmit={(outcome) =>
                  onHumanSubmit(pendingHumanQuestion.id, outcome)
                }
              />
            ) : (
              // Always-on composer. Renders for live work (queues into
              // the running loop) AND for a closed session (re-seeds a
              // fresh run) so Nexie is never a dead end — replaces the
              // old "hide the box on terminal runs" behaviour that
              // stranded the operator on "Session ended." embedded:
              // queued messages are folded into the transcript above.
              session.runId && (
                <AgentChatbox
                  runId={session.runId}
                  embedded
                  placeholder={composerPlaceholder(session.runStatus)}
                  onSend={onComposerSend}
                />
              )
            )}
          </div>
        )}
    </div>
  );
}
