import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "wouter";
import { ExternalLinkIcon } from "@radix-ui/react-icons";

import {
  DEFAULT_WHATS_NEXT_BOT_ID,
  getFirstClassBot,
  recentBoardSummary,
  recentCreatedIssueIds,
  titleForIssueId,
} from "@/lib/whats-next/firstClassBots";
import { useWhatsNextSession } from "@/lib/whats-next/useWhatsNextSession";
import type { FormAnswer, FormSpec } from "@/lib/whats-next/questionForm";
import type {
  DispatchCandidatesMessage,
  IssuesSummaryMessage,
  RoadmapDoc,
  WhatsNextMessage,
} from "@/lib/whats-next/messages";

import { cancelRun } from "@/api/runs";
import AgentChatbox from "@/components/shared/AgentChatbox";
import { Button } from "@/components/ui/Button";
import { Select, Textarea } from "@/components/ui";
import { labelForStatus } from "@/components/Runs/runStatusMeta";
import {
  classifyContinueIntent,
  type ContinueAction,
} from "@/lib/whats-next/classifyContinueIntent";
import { useUIStore } from "@/store/ui";
import ChatTranscript from "./ChatTranscript";
import HumanChatTurn from "./HumanChatTurn";
import PreFlightPanel from "./PreFlightPanel";
import SessionLauncher from "./SessionLauncher";
import WatchPanel from "./WatchPanel";

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
      if (typeof console !== "undefined") {
        console.debug("[whats-next] onHumanSubmit", {
          messageId,
          outcome,
          hasBot: !!bot,
          hasMessage: !!m,
          messageKind: m?.kind,
          messageStatus: m?.kind === "human-question" ? m.status : undefined,
          hasEntry: !!entry,
          runId: session.runId,
        });
      }
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
  // excludeMessageId to ChatTranscript.
  const pendingHumanQuestion = session.messages.find(
    (m): m is Extract<typeof m, { kind: "human-question" }> =>
      m.kind === "human-question" && m.status === "pending",
  );
  const pendingForm = pendingHumanQuestion
    ? resolveDynamicForm(pendingHumanQuestion, session.messages, bot.nodeMap)
    : undefined;

  // Auto-submit the stashed launcher form answer into the first
  // pending human-question whose node id matches launcherFormTarget.
  // The operator picked their priority before the bot ran; once
  // explore finishes and ask_priorities surfaces, we resolve it
  // silently rather than asking again.
  useEffect(() => {
    if (!pendingHumanQuestion) return;
    if (!bot.launcherFormTarget) return;
    if (pendingHumanQuestion.nodeId !== bot.launcherFormTarget) return;
    const stash = pendingLauncherAnswer.current;
    if (!stash) return;
    pendingLauncherAnswer.current = null;
    void session.submitHumanAnswer(pendingHumanQuestion.id, stash);
  }, [pendingHumanQuestion, bot.launcherFormTarget, session]);

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
              <div className="border-t border-danger/40 bg-danger-soft px-4 py-2 text-[12px] text-danger-fg">
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
              session.runId &&
              session.runStatus !== "finished" &&
              session.runStatus !== "failed" && (
                // embedded={true}: queued messages are folded into the
                // transcript above (as user-message cards), so the
                // chatbox suppresses its own duplicate list. Only the
                // composer + skill picker remain.
                <AgentChatbox runId={session.runId} embedded />
              )
            )}
          </div>
        )}
    </div>
  );
}

// resolveDynamicForm overrides the static nodeMap.form for nodes
// whose options depend on upstream output:
//   - ask_which_to_process: a checkbox per freshly-created issue
//     (from the IssuesSummaryMessage emit_action just pushed).
//   - human_review: a checkbox per proposed roadmap_item (from the
//     latest RoadmapCardMessage). Lets the operator drop individual
//     items before emit_action materialises them as kanban issues,
//     instead of having to create everything then close the ones they
//     didn't want.
// Falls back to the static nodeMap.form when the upstream message
// isn't available — under normal flow it always is by the time the
// human pause hits.
function resolveDynamicForm(
  message: Extract<WhatsNextMessage, { kind: "human-question" }>,
  messages: WhatsNextMessage[],
  nodeMap: Record<string, { form?: FormSpec } | undefined>,
): FormSpec | undefined {
  const staticForm = nodeMap[message.nodeId]?.form;
  const pendingIdx = messages.findIndex((m) => m.id === message.id);
  const upstream = pendingIdx < 0 ? messages : messages.slice(0, pendingIdx);

  if (message.nodeId === "ask_which_to_process") {
    const summary = findLatestIssuesSummary(upstream);
    if (!summary || summary.createdIssues.length === 0) return staticForm;
    return {
      mode: "flat",
      questions: [
        {
          id: "selected_issue_ids",
          kind: "checkbox",
          label: "Issues to dispatch now",
          description:
            "Tick the ones to push from backlog to ready. The dispatcher picks up ready tickets matching the configured assignee_workflows mapping.",
          options: summary.createdIssues.map((iss) => ({
            value: iss.id,
            label: iss.title,
            description: [iss.horizon, iss.assignee]
              .filter(Boolean)
              .join(" · "),
          })),
          defaultValues: summary.createdIssues.map((iss) => iss.id),
        },
        {
          id: "note",
          kind: "free_text",
          label: "Note (optional)",
          description:
            "Why this selection? Helps the bot reason about edge cases.",
          placeholder:
            "Optional — e.g. 'skip long_term, only ship next_action'",
          rows: 2,
          required: false,
        },
      ],
      submitLabel: "Dispatch selected",
    };
  }

  if (message.nodeId === "ask_which_to_dispatch_more") {
    const candidates = findLatestDispatchCandidates(upstream);
    if (!candidates || candidates.candidates.length === 0) return staticForm;
    return {
      mode: "flat",
      questions: [
        {
          id: "selected_issue_ids",
          kind: "checkbox",
          label: "Items to push to ready",
          description:
            "Tick what should move into ready next. Items already in ready are shown so you can re-confirm them if the dispatcher hasn't claimed them yet.",
          options: candidates.candidates.map((c) => ({
            value: c.id,
            label: c.title,
            description: [c.state, c.assignee || c.bot]
              .filter(Boolean)
              .join(" · "),
          })),
          // Pre-tick nothing — unlike ask_which_to_process where the
          // operator just emitted these items and wants them all,
          // here we're showing the whole eligible board and a
          // default-all selection would silently dispatch the long
          // tail. Make the operator pick.
          defaultValues: [],
        },
        {
          id: "note",
          kind: "free_text",
          label: "Note (optional)",
          description: "Optional — context for downstream nodes.",
          placeholder: "e.g. 'only the feature_dev items'",
          rows: 2,
          required: false,
        },
      ],
      submitLabel: "Dispatch selected",
    };
  }

  if (message.nodeId === "ask_continue") {
    const actionQ = (staticForm?.questions ?? [])[0];
    const detailQ = (staticForm?.questions ?? [])[1];
    if (!actionQ || actionQ.kind !== "radio") return staticForm;

    const created = recentCreatedIssueIds(upstream);
    const titles: string[] = created
      .map((id: string) => titleForIssueId(upstream, id))
      .filter((t): t is string => typeof t === "string" && t.length > 0);

    // UX#1: when the previous turn created tickets, prepend the
    // "dispatch what I just created" shortcut and default to it.
    if (created.length > 0) {
      const shortcutLabel =
        titles.length === 0
          ? `Dispatch what I just created (${created.length})`
          : titles.length === 1
            ? `Dispatch ${titles[0]}`
            : titles.length <= 3
              ? `Dispatch: ${titles.join(", ")}`
              : `Dispatch the ${titles.length} I just created`;
      return {
        ...staticForm,
        mode: "flat",
        questions: [
          {
            ...actionQ,
            options: [
              {
                value: "dispatch_just_created",
                label: shortcutLabel,
                description:
                  "Push the ticket(s) you added in the previous turn from backlog to ready. The dispatcher picks them up immediately.",
              },
              ...actionQ.options,
            ],
            defaultValue: "dispatch_just_created",
          },
          ...(detailQ ? [detailQ] : []),
        ],
        submitLabel: staticForm?.submitLabel,
      } as FormSpec;
    }

    // UX#2: no fresh tickets this turn — smart-default the radio to
    // the next-likely action based on what the operator did last
    // loop. Saves a click on the common chains. `done` is never
    // auto-selected: ending the session must be an explicit pick.
    const smartDefault = smartContinueDefault(previousContinueAction(upstream));
    if (!smartDefault) return staticForm;
    return {
      ...staticForm,
      mode: "flat",
      questions: [
        { ...actionQ, defaultValue: smartDefault },
        ...(detailQ ? [detailQ] : []),
      ],
      submitLabel: staticForm?.submitLabel,
    } as FormSpec;
  }

  if (message.nodeId === "human_review") {
    const roadmap = findLatestRoadmap(upstream);
    if (!roadmap) return staticForm;
    const items = flattenRoadmapItems(roadmap);
    if (items.length === 0) return staticForm;
    return {
      mode: "flat",
      questions: [
        {
          id: "selected_titles",
          kind: "checkbox",
          label: "Items to actually create as kanban issues",
          description:
            "Pre-ticked = approve all. Untick any you don't want emit_action to materialise on the board right now; rationale + acceptance criteria still survive in the audit markdown.",
          options: items.map((it) => ({
            value: it.title,
            label: it.title,
            description: [it.horizon, it.assignee]
              .filter(Boolean)
              .join(" · "),
          })),
          defaultValues: items.map((it) => it.title),
          required: false,
        },
      ],
    };
  }

  return staticForm;
}

// flattenRoadmapItems concatenates the three horizons into a single
// list while tagging each item with its horizon — the operator's
// checkbox column should treat next_action and long_term items
// uniformly; the horizon survives as option meta so the badge is
// still visible.
function flattenRoadmapItems(
  roadmap: RoadmapDoc,
): Array<{ title: string; assignee: string; horizon: string }> {
  const out: Array<{ title: string; assignee: string; horizon: string }> = [];
  for (const it of roadmap.long_term) {
    if (it && typeof it.title === "string" && it.title.length > 0) {
      out.push({ title: it.title, assignee: it.assignee ?? "", horizon: "long_term" });
    }
  }
  for (const it of roadmap.short_term) {
    if (it && typeof it.title === "string" && it.title.length > 0) {
      out.push({ title: it.title, assignee: it.assignee ?? "", horizon: "short_term" });
    }
  }
  if (
    roadmap.next_action &&
    typeof roadmap.next_action.title === "string" &&
    roadmap.next_action.title.length > 0
  ) {
    out.push({
      title: roadmap.next_action.title,
      assignee: roadmap.next_action.assignee ?? "",
      horizon: "next_action",
    });
  }
  return out;
}

function findLatestRoadmap(messages: WhatsNextMessage[]): RoadmapDoc | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m && m.kind === "roadmap-card") return m.roadmap;
  }
  return null;
}

function findLatestIssuesSummary(
  messages: WhatsNextMessage[],
): IssuesSummaryMessage | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m && m.kind === "issues-summary") return m;
  }
  return null;
}

function findLatestDispatchCandidates(
  messages: WhatsNextMessage[],
): DispatchCandidatesMessage | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m && m.kind === "dispatch-candidates") return m;
  }
  return null;
}

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

// contextPrefixFor returns a one-line "what just happened" hint to
// render above the pending form, so the operator doesn't have to
// scroll up to remember the previous action's outcome. Today only
// ask_continue uses it (after a triage_board turn); other human
// nodes fall through to the empty string and the footer renders
// without a prefix.
function contextPrefixFor(
  message: Extract<WhatsNextMessage, { kind: "human-question" }>,
  messages: WhatsNextMessage[],
): string {
  if (message.nodeId !== "ask_continue") return "";
  const pendingIdx = messages.findIndex((m) => m.id === message.id);
  const upstream = pendingIdx < 0 ? messages : messages.slice(0, pendingIdx);
  return recentBoardSummary(upstream);
}

// PendingTurnFooter wraps HumanChatTurn in a Claude-Code-style fixed
// footer: top border, slightly stronger surface, comfortable padding.
// The wrapped HumanChatTurn keeps all its existing rendering (form /
// free-text / actions) — only the surround changes.
function PendingTurnFooter({
  message,
  form,
  busy,
  contextPrefix,
  onSubmit,
}: {
  message: Parameters<typeof HumanChatTurn>[0]["message"];
  form: Parameters<typeof HumanChatTurn>[0]["form"];
  busy: boolean;
  contextPrefix?: string;
  onSubmit: Parameters<typeof HumanChatTurn>[0]["onSubmit"];
}) {
  return (
    <div
      className="border-t border-border-default bg-surface-1"
      role="status"
      aria-live="polite"
    >
      <div className="mx-auto max-w-3xl px-4 py-3">
        {contextPrefix && contextPrefix.length > 0 && (
          <div className="mb-2 text-[11px] text-fg-muted italic">
            {contextPrefix}
          </div>
        )}
        <HumanChatTurn
          message={message}
          form={form}
          busy={busy}
          onSubmit={onSubmit}
        />
      </div>
    </div>
  );
}

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
  modify_ticket: "Modify a ticket",
  dispatch_more: "Dispatch more",
  done: "End the session",
};

function QuickModeFooter({
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

  const submit = () => {
    if (!ready || busy) return;
    // ask_continue's schema is {action, detail}; the bot's
    // derive_continue keys on action verbatim. "done" needs no detail.
    onSubmit({ action, detail: action === "done" ? "" : detail });
  };

  return (
    <div
      className="border-t border-border-default bg-surface-1"
      role="status"
      aria-live="polite"
    >
      <div className="mx-auto max-w-3xl px-4 py-3 space-y-2">
        {contextPrefix && contextPrefix.length > 0 && (
          <div className="text-[11px] text-fg-muted italic">{contextPrefix}</div>
        )}
        <Textarea
          rows={2}
          value={raw}
          placeholder="What's next? e.g. “dispatch the feature_dev tickets”, “add a ticket for the flaky sandbox boot”, “done”."
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
              <span className="text-[10px] text-fg-subtle uppercase tracking-wide shrink-0">
                I'll
              </span>
              <Select
                value={action}
                onChange={(e) =>
                  setActionOverride(e.target.value as ContinueAction)
                }
                className="text-[11px] py-0.5"
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
            {action !== "done" && (
              <input
                type="text"
                value={detail}
                onChange={(e) => setDetailOverride(e.target.value)}
                placeholder="detail (optional)"
                className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-[11px] text-fg-default"
              />
            )}
            {lowConfidence && (
              <div className="text-[10px] text-warning-fg">
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
            className="text-[11px] text-fg-subtle hover:text-fg-default cursor-pointer"
            title="Switch back to the structured action + detail form."
          >
            Use form instead
          </button>
        </div>
      </div>
    </div>
  );
}

// ResumeFooter is the bottom-of-chat call-to-action when the run is
// in failed_resumable or cancelled state. Replaces the AgentChatbox
// (which is hidden for terminal runs) so the operator's next action
// is obvious: re-enter the run from its last checkpoint.
function ResumeFooter({
  runStatus,
  busy,
  onResume,
}: {
  runStatus: "failed_resumable" | "cancelled";
  busy: boolean;
  onResume: () => void;
}) {
  const explainer =
    runStatus === "failed_resumable"
      ? "A step failed. The run kept its checkpoint — Resume retries from that point."
      : "Run was cancelled. Resume picks up from the last checkpoint.";
  return (
    <div className="border-t border-border-default bg-surface-1">
      <div className="mx-auto max-w-3xl px-4 py-3 flex items-center gap-3">
        <div className="flex-1 text-[12px] text-fg-muted">{explainer}</div>
        <Button
          variant="primary"
          size="sm"
          disabled={busy}
          onClick={onResume}
        >
          {busy ? "…" : "Resume"}
        </Button>
      </div>
    </div>
  );
}

function SessionHeader({
  bot,
  session,
}: {
  bot: { label: string };
  session: ReturnType<typeof useWhatsNextSession>;
}) {
  const [abandoning, setAbandoning] = useState(false);
  // A session is "live" when it has an in-flight run that hasn't reached
  // a terminal state. Abandoning a live run must cancel it server-side
  // before resetting the UI — otherwise newSession just orphans the
  // engine goroutine, which keeps burning model spend until something
  // else (stall watchdog, process restart) tears it down.
  const isLive =
    session.runId !== null &&
    session.status !== "ended" &&
    session.status !== "idle";

  const onNewSession = useCallback(async () => {
    if (isLive) {
      const ok = window.confirm(
        "The current Nexie session is still running. Cancel it and start a new one?",
      );
      if (!ok) return;
      setAbandoning(true);
      try {
        if (session.runId) {
          await cancelRun(session.runId);
        }
      } catch {
        // Surface but don't block: even if cancel races (e.g. the run
        // just finished), the reset below still lands the user on a
        // fresh launcher; the worst case is a quiescent orphan that
        // the existing stall sweep will reconcile.
      } finally {
        setAbandoning(false);
      }
    }
    session.newSession();
  }, [isLive, session]);

  // The button is hidden when there's nothing to reset (no runId yet,
  // pre-launch). Otherwise it stays available across every run state
  // so the operator can always escape — the prior behaviour gated it
  // on `status === "ended"`, which trapped them inside paused or
  // failed_resumable sessions.
  const showResetButton = session.runId !== null && session.status !== "launching";

  return (
    <div className="px-4 py-3 border-b border-border-subtle flex items-baseline justify-between gap-3">
      <h2 className="text-[13px] font-semibold text-fg-default">
        {bot.label}
        {session.runId && (
          <span className="ml-2 text-[10px] text-fg-subtle font-mono font-normal">
            {session.runId}
          </span>
        )}
      </h2>
      <div className="flex items-baseline gap-3">
        <QuickModeToggle />
        {session.runId && (
          <Link
            href={`/runs/${encodeURIComponent(session.runId)}`}
            className="inline-flex items-center gap-1 text-[11px] text-accent hover:underline"
          >
            <ExternalLinkIcon className="w-3 h-3" />
            console
          </Link>
        )}
        <div className="text-[10px] uppercase tracking-wide text-fg-subtle">
          {humanStatus(session.status, session.runStatus)}
        </div>
        {showResetButton && (
          <button
            type="button"
            onClick={() => void onNewSession()}
            disabled={abandoning}
            className="text-[11px] text-accent hover:underline cursor-pointer disabled:opacity-50 disabled:cursor-wait"
            title={
              isLive
                ? "Cancel the current run and start a fresh Nexie session."
                : "Start fresh — the current run stays in the run list."
            }
          >
            {abandoning ? "Cancelling…" : isLive ? "Abandon & restart" : "New session"}
          </button>
        )}
      </div>
    </div>
  );
}

// QuickModeToggle is the SessionHeader control that flips the
// ask_continue footer between the structured action+detail form and
// the free-text Quick mode. Persisted via the ui store so the
// operator's preference survives reloads + sessions.
function QuickModeToggle() {
  const quickMode = useUIStore((s) => s.whatsNextQuickMode);
  const setQuickMode = useUIStore((s) => s.setWhatsNextQuickMode);
  return (
    <button
      type="button"
      onClick={() => setQuickMode(!quickMode)}
      className={`text-[11px] hover:underline cursor-pointer ${
        quickMode ? "text-accent" : "text-fg-subtle"
      }`}
      title="Quick mode: type a free-text instruction on the ask_continue turn instead of picking from the form. A dry-run banner lets you confirm the interpreted action."
    >
      {quickMode ? "⚡ Quick mode" : "Quick mode"}
    </button>
  );
}

export function humanStatus(
  hi: ReturnType<typeof useWhatsNextSession>["status"],
  raw: ReturnType<typeof useWhatsNextSession>["runStatus"],
): string {
  if (hi === "launching") return "Launching…";
  if (hi === "submitting") return "Submitting…";
  if (hi === "ended") {
    const label = raw ? labelForStatus(raw) : "unknown";
    return `Ended · ${label}`;
  }
  if (raw === "paused_waiting_human") return "Waiting for your reply";
  return "Active";
}
