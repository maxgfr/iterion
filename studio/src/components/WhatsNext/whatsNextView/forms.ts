import {
  recentBoardSummary,
  recentCreatedIssueIds,
  titleForIssueId,
} from "@/lib/whats-next/firstClassBots";
import type { FormSpec } from "@/lib/whats-next/questionForm";
import type {
  DispatchCandidatesMessage,
  IssuesSummaryMessage,
  RoadmapDoc,
  WhatsNextMessage,
} from "@/lib/whats-next/messages";

import {
  previousContinueAction,
  smartContinueDefault,
} from "./smartContinue";

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
export function resolveDynamicForm(
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

// contextPrefixFor returns a one-line "what just happened" hint to
// render above the pending form, so the operator doesn't have to
// scroll up to remember the previous action's outcome. Today only
// ask_continue uses it (after a triage_board turn); other human
// nodes fall through to the empty string and the footer renders
// without a prefix.
export function contextPrefixFor(
  message: Extract<WhatsNextMessage, { kind: "human-question" }>,
  messages: WhatsNextMessage[],
): string {
  if (message.nodeId !== "ask_continue") return "";
  const pendingIdx = messages.findIndex((m) => m.id === message.id);
  const upstream = pendingIdx < 0 ? messages : messages.slice(0, pendingIdx);
  return recentBoardSummary(upstream);
}
