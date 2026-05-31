// First-class bot registry — bots that get a dedicated /whats-next
// experience instead of being launched generically through LaunchView.
//
// v0 hard-codes the single whats-next entry. When a second first-class
// bot lands, promote this registry to a manifest-driven discovery (or
// a server-side endpoint), and replace the const with a fetch.
//
// `nodeMap` describes how each node id of the workflow renders in the
// WhatsNext chat. The keys must match the `.bot` source — a rename there
// without updating the map silently drops the node from the chat.

import { shortIssueId } from "./issueId";
import type { FormSpec } from "./questionForm";

export type WhatsNextNodeKind =
  | "banner"
  | "human"
  | "silent"
  | "issues-summary"
  | "roadmap";

// Walk upstream chat messages back-to-front for the most recent
// triage-summary card and return its createdIssueIds. Handles both
// the in-flight ExtensionMessage shape (kind="extension", tag) and
// the post-processed typed shape (kind="triage-summary") — the
// formatter runs during fold, before postProcess promotes the
// extension to the typed kind, so both windows must match.
export function recentCreatedIssueIds(
  upstream?: ReadonlyArray<unknown>,
): string[] {
  if (!upstream || upstream.length === 0) return [];
  for (let i = upstream.length - 1; i >= 0; i--) {
    const m = upstream[i] as
      | { kind?: string; tag?: string; payload?: unknown; createdIssueIds?: unknown }
      | null;
    if (!m || typeof m !== "object") continue;
    const obj = m as Record<string, unknown>;
    if (obj.kind === "triage-summary") {
      const ids = obj.createdIssueIds;
      return Array.isArray(ids)
        ? ids.filter((x): x is string => typeof x === "string")
        : [];
    }
    if (obj.kind === "extension" && obj.tag === "triage-summary") {
      const p = obj.payload as { createdIssueIds?: unknown } | undefined;
      const ids = p?.createdIssueIds;
      return Array.isArray(ids)
        ? ids.filter((x): x is string => typeof x === "string")
        : [];
    }
  }
  return [];
}

// recentBoardSummary returns the most recent triage_board's
// board_summary text — used by the WhatsNextView contextual prompt
// to remind the operator what just happened above the next form.
export function recentBoardSummary(
  upstream?: ReadonlyArray<unknown>,
): string {
  if (!upstream || upstream.length === 0) return "";
  for (let i = upstream.length - 1; i >= 0; i--) {
    const m = upstream[i] as
      | { kind?: string; tag?: string; payload?: unknown; boardSummary?: unknown }
      | null;
    if (!m || typeof m !== "object") continue;
    const obj = m as Record<string, unknown>;
    if (obj.kind === "triage-summary" && typeof obj.boardSummary === "string") {
      return obj.boardSummary;
    }
    if (obj.kind === "extension" && obj.tag === "triage-summary") {
      const p = obj.payload as { boardSummary?: unknown } | undefined;
      if (p && typeof p.boardSummary === "string") return p.boardSummary;
    }
  }
  return "";
}

// titleForIssueId walks upstream for any card carrying issue cards
// (issues-summary's createdIssues, dispatch-candidates's candidates)
// and returns the matching title. Returns null when nothing matches.
// Used by the dispatch_just_created formatter so the answered turn
// reads "Dispatch: <title>" rather than the opaque id.
export function titleForIssueId(
  upstream: ReadonlyArray<unknown> | undefined,
  issueId: string,
): string | null {
  if (!upstream || upstream.length === 0) return null;
  for (let i = upstream.length - 1; i >= 0; i--) {
    const m = upstream[i] as Record<string, unknown> | null;
    if (!m || typeof m !== "object") continue;
    const candidates = liftIssueRows(m);
    for (const row of candidates) {
      if (row.id === issueId) return row.title;
    }
  }
  return null;
}

function liftIssueRows(
  m: Record<string, unknown>,
): Array<{ id: string; title: string }> {
  let list: unknown = undefined;
  if (m.kind === "issues-summary") list = m.createdIssues;
  else if (m.kind === "dispatch-candidates") list = m.candidates;
  else if (m.kind === "extension") {
    const tag = m.tag;
    const p = m.payload as Record<string, unknown> | undefined;
    if (tag === "issues-summary") list = p?.createdIssues;
    else if (tag === "dispatch-candidates") list = p?.candidates;
  }
  if (!Array.isArray(list)) return [];
  return list
    .filter(
      (r): r is Record<string, unknown> =>
        !!r &&
        typeof r === "object" &&
        typeof (r as Record<string, unknown>).id === "string" &&
        typeof (r as Record<string, unknown>).title === "string",
    )
    .map((r) => ({ id: r.id as string, title: r.title as string }));
}

export interface WhatsNextNodeMapEntry {
  kind: WhatsNextNodeKind;
  // Label shown in the progress banner ("Surveying repository…").
  label?: string;
  // For agent nodes whose output should be promoted to a typed card
  // after the banner closes. Each kind has its own renderer.
  followCardKind?:
    | "roadmap"
    | "issuesSummary"
    | "survey"
    | "dispatchCandidates"
    | "triageSummary";
  // For "banner" entries: pluck this field from the node output as the
  // collapsed summary text. Optional — if absent, the banner closes
  // without a summary line.
  summaryField?: string;
  // For "human" entries: the assistant-side prompt displayed above the
  // user input. Static for v0; future iterations may pull from the
  // node's `instructions:` block.
  prompt?: string;
  // For "human" entries with custom actions (e.g. human_review's
  // approve/request_revision). Default: free-text submit.
  actions?: ReadonlyArray<"approve" | "request_revision">;
  // For "human" entries: the schema field name where the user's typed
  // text lands. ask_priorities → "context"; human_review → "feedback".
  // Required for kind "human" when no `form` is set (the legacy
  // textarea-only path).
  textField?: string;
  // For "human" entries with approve/reject buttons: the schema field
  // name for the boolean outcome. human_review → "approved". Optional —
  // free-text-only turns leave this unset.
  approvedField?: string;
  // For "human" entries: a rich form specification (radio / checkbox /
  // select / free_text, with optional "Other"). When set, the
  // HumanChatTurn renders the form via WizardForm and the form
  // answers are submitted as-is (question.id IS the answer key). When
  // unset, the legacy single-textarea + optional approve/reject UI
  // kicks in.
  form?: FormSpec;
  // For "human" entries with multi-question forms (action + detail,
  // radio + free_text, …): synthesise the AnsweredTurn label from the
  // full answers map. Without this, the resolver picks only textField
  // and a turn with action="dispatch_more" + detail="" renders as
  // "(empty reply)" — visually erasing the operator's choice. Return
  // an empty string to fall back to the textField/generic path.
  //
  // The optional `upstream` array carries the chat messages preceding
  // this turn. Use it to resolve checkbox-ID answers back to titles
  // — the ask_which_to_dispatch_more formatter joins selected IDs to
  // human-readable issue titles via the upstream DispatchCandidatesMessage.
  formatAnswer?: (
    answers: Record<string, unknown>,
    upstream?: ReadonlyArray<unknown>,
  ) => string;
}

export interface FirstClassBot {
  id: string;
  label: string;
  description: string;
  // Path relative to the server's work_dir. Resolved at launch time.
  workflowPath: string;
  // Vars to expose in the SessionLauncher with pre-fill rules.
  launcherVars: ReadonlyArray<{
    name: string;
    label: string;
    defaultFrom?: "work_dir";
  }>;
  // Optional upfront form rendered by SessionLauncher instead of the
  // bare Start button. When set, its answer is stashed and auto-
  // submitted into the chat's first matching human turn (see
  // `launcherFormTarget` for the wire-up). Operators land on a
  // pickable question instead of a button they have to "Start" first.
  launcherForm?: FormSpec;
  // Node id whose first pending human-question receives the launcher
  // form's answer. Required when launcherForm is set.
  launcherFormTarget?: string;
  // When true, SessionLauncher renders a secondary "Dispatch existing
  // board items" button that launches with vars.mode="dispatch_only".
  // The bot's workflow is responsible for branching on that var (e.g.
  // a classify_entry compute that routes to load_dispatch_candidates
  // when set). Bots without this routing should leave it false.
  supportsDispatchOnly?: boolean;
  nodeMap: Readonly<Record<string, WhatsNextNodeMapEntry>>;
}

// formatPickedIssues resolves a checkbox answer (selected_issue_ids
// as a JSON array of opaque board IDs) back into the human-readable
// titles the operator just ticked. Looks at the most recent upstream
// card of the given kind ("issues-summary" for emit_action's freshly-
// created issues, "dispatch-candidates" for the board-snapshot picker)
// and joins the matched titles. Falls back to a count + truncated-id
// digest when the card or titles are missing — never returns the bare
// "native:<uuid>" form that's unreadable in the chat strip.
function formatPickedIssues(
  answers: Record<string, unknown>,
  upstream: ReadonlyArray<unknown> | undefined,
  cardKind: "issues-summary" | "dispatch-candidates",
): string {
  const raw = answers["selected_issue_ids"];
  if (typeof raw === "string") return raw; // legacy CLI free-text path
  if (!Array.isArray(raw) || raw.length === 0) return "";
  const ids = raw.filter((v): v is string => typeof v === "string");
  const titleById = collectTitleMap(upstream, cardKind);
  const titles = ids
    .map((id) => titleById.get(id))
    .filter((t): t is string => typeof t === "string" && t.length > 0);
  if (titles.length === ids.length && ids.length > 0) {
    if (ids.length === 1) return titles[0] ?? "";
    if (ids.length <= 3) return titles.join(", ");
    return `${ids.length} items: ${titles.slice(0, 2).join(", ")}, …`;
  }
  // Partial / no match: surface what we can. Truncated IDs are
  // strictly more informative than the full opaque UUID.
  const truncated = ids.map(shortIssueId);
  if (ids.length === 1) return truncated[0] ?? "";
  return `${ids.length} items: ${truncated.join(", ")}`;
}

// collectTitleMap scans the upstream chat messages for the most
// recent card of the given kind and lifts {id → title} pairs from
// it. Returns an empty map when none exists — the caller then falls
// back to the truncated-id rendering. Defensive against unknown
// shapes: a missing field / wrong type silently skips that entry
// rather than throwing.
//
// Critical: humanAnswerExtractor runs DURING the fold, before the
// whats-next resolver's postProcess lifts ExtensionMessage entries
// into their typed shapes (RoadmapCardMessage, IssuesSummaryMessage,
// DispatchCandidatesMessage). So the upstream array at this point
// carries `kind: "extension"` with the original tag, not the typed
// `kind` the rendered transcript shows. We match on the extension
// tag here AND on the post-processed kind so the formatter works
// whether it's called during the initial fold or against a fully
// post-processed message stream (defensive for future callers).
function collectTitleMap(
  upstream: ReadonlyArray<unknown> | undefined,
  cardKind: "issues-summary" | "dispatch-candidates",
): Map<string, string> {
  const out = new Map<string, string>();
  if (!upstream || upstream.length === 0) return out;
  // Walk back-to-front: the latest matching card wins on ID
  // collisions (e.g. two emit_action passes in the same session).
  for (let i = upstream.length - 1; i >= 0; i--) {
    const m = upstream[i] as
      | { kind?: string; tag?: string; payload?: unknown }
      | { kind?: string; createdIssues?: unknown; candidates?: unknown }
      | null;
    if (!m || typeof m !== "object") continue;
    const obj = m as Record<string, unknown>;
    let list: unknown = undefined;
    if (obj.kind === cardKind) {
      // Post-processed typed card.
      list =
        cardKind === "issues-summary" ? obj.createdIssues : obj.candidates;
    } else if (obj.kind === "extension" && obj.tag === cardKind) {
      // In-flight ExtensionMessage — its payload mirrors the typed
      // shape because the whats-next resolver builds it that way.
      const payload = obj.payload as Record<string, unknown> | undefined;
      if (payload) {
        list =
          cardKind === "issues-summary"
            ? payload.createdIssues
            : payload.candidates;
      }
    } else {
      continue;
    }
    if (!Array.isArray(list)) continue;
    for (const row of list) {
      if (!row || typeof row !== "object") continue;
      const id = (row as Record<string, unknown>).id;
      const title = (row as Record<string, unknown>).title;
      if (typeof id === "string" && typeof title === "string") {
        out.set(id, title);
      }
    }
    return out;
  }
  return out;
}

// askPrioritiesForm is the priorities radio + free-text question.
// Defined once and referenced from both the SessionLauncher (where it
// captures the operator's intent upfront) and the ask_priorities
// nodeMap entry (the bot's matching human turn, auto-submitted with
// the launcher's answer when present).
const askPrioritiesForm: FormSpec = {
  questions: [
    {
      id: "context",
      kind: "radio",
      label: "What matters right now?",
      description:
        "Pick a focus or type your own. Iterion will draft a roadmap based on this.",
      options: [
        {
          value: "Ship a specific feature",
          label: "Ship a specific feature",
          description: "There's a feature I want delivered next.",
        },
        {
          value: "Fix bugs / pay down tech debt",
          label: "Fix bugs / pay down tech debt",
          description: "Stabilise before adding more.",
        },
        {
          value: "Explore — surface what's important",
          label: "Explore — surface what's important",
          description: "I want iterion to propose, not me.",
        },
        {
          value: "Polish & refinement",
          label: "Polish & refinement",
          description:
            "Smooth the rough edges in what's already shipped — docs, ergonomics, edge cases, perf.",
        },
      ],
      allow_other: true,
    },
  ],
  submitLabel: "Start",
};

export const FIRST_CLASS_BOTS: Readonly<Record<string, FirstClassBot>> = {
  "whats-next": {
    id: "whats-next",
    label: "What's Next",
    description:
      "Decide what to push on the board next. Iterion surveys the repo, drafts a roadmap, you approve, and it materialises as kanban issues the dispatcher can dispatch.",
    workflowPath: "examples/whats-next/main.bot",
    // Studio launches scope to the server's current work_dir, so the
    // bot's `workspace_dir` var resolves to the same path via its
    // `${PROJECT_DIR}` default. No launcher vars needed.
    launcherVars: [],
    launcherForm: askPrioritiesForm,
    launcherFormTarget: "ask_priorities",
    supportsDispatchOnly: true,
    nodeMap: {
      explore: {
        kind: "banner",
        label: "Surveying the repository",
        followCardKind: "survey",
      },
      ask_priorities: {
        kind: "human",
        prompt: "What matters right now?",
        textField: "context",
        // Rich form: radio with a few common focuses plus an "Other"
        // free-text fallback. The chosen value lands in `context`
        // either as one of the option strings or as the typed text —
        // both satisfy the priorities_output schema (context: string).
        // Shared with the SessionLauncher's launcherForm, which
        // captures the answer upfront and auto-submits this turn.
        form: askPrioritiesForm,
      },
      propose_roadmap: {
        kind: "banner",
        label: "Drafting the roadmap",
        followCardKind: "roadmap",
      },
      carry_roadmap: { kind: "silent" },
      human_review: {
        kind: "human",
        prompt: "Review the proposed roadmap.",
        actions: ["approve", "request_revision"],
        textField: "feedback",
        approvedField: "approved",
      },
      revise_roadmap: {
        kind: "banner",
        label: "Revising the roadmap",
        followCardKind: "roadmap",
      },
      emit_action: {
        kind: "banner",
        label: "Creating kanban issues",
        followCardKind: "issuesSummary",
      },
      // Post-emit triage loop: ask the operator which freshly-created
      // issues to dispatch, hand them off to assign_to_bots, then
      // loop on ask_continue / triage_board until the operator
      // picks action=done.
      // The form for this node is built DYNAMICALLY from the previous
      // IssuesSummaryMessage (one checkbox per freshly-created issue).
      // WhatsNextView's footer renderer constructs it at message time;
      // the static form below is a fallback for tests / cases where the
      // upstream summary message is missing.
      ask_which_to_process: {
        kind: "human",
        prompt:
          "Pick which issues to push from backlog to ready (the dispatcher will pick them up).",
        textField: "selected_issue_ids",
        // The dynamic checkbox form submits selected_issue_ids as a
        // JSON array, not a string — without this formatter the
        // generic textField extractor (which only honors strings)
        // falls back to "(empty reply)" even when the operator
        // ticked one or more boxes. We resolve the IDs back to
        // human-readable titles via the upstream IssuesSummaryMessage
        // so the AnsweredTurn reads "Add whats-next board-dispatch
        // smoke loop" instead of "native:e2b975b9".
        formatAnswer: (answers, upstream) =>
          formatPickedIssues(answers, upstream, "issues-summary"),
        form: {
          questions: [
            {
              id: "selected_issue_ids",
              kind: "free_text",
              label: "Issue IDs to dispatch (fallback)",
              description:
                'JSON array of IDs, "all", or leave empty. Normally rendered as a checkbox list — this free-text shows only when the upstream summary message is missing.',
              placeholder: '["abc12345","def67890"] or "all"',
              rows: 3,
              required: false,
            },
            {
              id: "note",
              kind: "free_text",
              label: "Note (optional)",
              description:
                "Why this selection? Helps the bot reason about edge cases.",
              placeholder: "Optional — e.g. 'skip long_term, only ship next_action'",
              rows: 2,
              required: false,
            },
          ],
          submitLabel: "Dispatch",
        },
      },
      assign_to_bots: {
        kind: "banner",
        label: "Moving issues to ready",
        summaryField: "summary",
      },
      // load_dispatch_candidates is the read-only board peek that
      // feeds ask_which_to_dispatch_more's checkbox list. It only
      // fires when the operator answered dispatch_more without
      // naming an item; otherwise it stays silent. We tag the
      // follow-card so the upstream candidates array lands in the
      // transcript as a typed message; resolveDynamicForm reads it
      // to build the checkbox column on the next human turn.
      load_dispatch_candidates: {
        kind: "banner",
        label: "Loading dispatchable items",
        summaryField: "summary",
        followCardKind: "dispatchCandidates",
      },
      // Mirror of ask_which_to_process: same checkbox UX, but the
      // candidates list comes from the load_dispatch_candidates
      // agent (current board state) instead of the freshly-created
      // emit_action issues. The dynamic form is built by
      // WhatsNextView.resolveDynamicForm at render time.
      ask_which_to_dispatch_more: {
        kind: "human",
        prompt:
          "Pick which board items to push to ready now — the dispatcher will pick them up.",
        textField: "selected_issue_ids",
        formatAnswer: (answers, upstream) =>
          formatPickedIssues(answers, upstream, "dispatch-candidates"),
        form: {
          questions: [
            {
              id: "selected_issue_ids",
              kind: "free_text",
              label: "Issue IDs to dispatch (fallback)",
              description:
                'Normally rendered as a checkbox list of current backlog + ready items — this free-text shows only when the upstream load_dispatch_candidates message is missing.',
              placeholder: '["abc12345","def67890"]',
              rows: 3,
              required: false,
            },
            {
              id: "note",
              kind: "free_text",
              label: "Note (optional)",
              description: "Optional — context to help downstream nodes.",
              placeholder: "e.g. 'only the feature_dev items'",
              rows: 2,
              required: false,
            },
          ],
          submitLabel: "Dispatch",
        },
      },
      // Two-field form rendered flat (both questions on one page):
      // pick an action AND describe what you want, submit once.
      // Triage_board reads action+detail together; without the
      // detail the loop just kept asking "what do you want to
      // dispatch?" with no operator way to answer.
      ask_continue: {
        kind: "human",
        prompt: "What's next on the board?",
        textField: "detail",
        // Synthesise the AnsweredTurn label from action + detail —
        // textField alone (detail) leaves "(empty reply)" whenever
        // the operator picks dispatch_more / done without typing,
        // visually erasing the choice. Action is the primary signal;
        // detail is appended after an em-dash when present.
        formatAnswer: (answers, upstream) => {
          const actionRaw = answers["action"];
          const detailRaw = answers["detail"];
          const action = typeof actionRaw === "string" ? actionRaw.trim() : "";
          const detail = typeof detailRaw === "string" ? detailRaw.trim() : "";
          if (action === "dispatch_just_created") {
            const ids = recentCreatedIssueIds(upstream);
            const titles = ids
              .map((id) => titleForIssueId(upstream, id))
              .filter((t): t is string => typeof t === "string" && t.length > 0);
            if (titles.length === 0) return "Dispatch what I just created";
            if (titles.length === 1) return `Dispatch: ${titles[0]}`;
            if (titles.length <= 3) return `Dispatch: ${titles.join(", ")}`;
            return `Dispatch ${titles.length} just-created tickets`;
          }
          if (!action) return detail;
          return detail ? `${action} — ${detail}` : action;
        },
        form: {
          questions: [
            {
              id: "action",
              kind: "radio",
              label: "Pick an action",
              options: [
                {
                  value: "add_ticket",
                  label: "Add a ticket",
                  description: "Create a new ticket on the board.",
                },
                {
                  value: "modify_ticket",
                  label: "Modify a ticket",
                  description:
                    "Re-assign, re-label, move, or close existing tickets.",
                },
                {
                  value: "dispatch_more",
                  label: "Dispatch more",
                  description:
                    'Push more backlog tickets to ready. Detail = "all", a list of IDs, or a filter like "feature_dev" / "short-term". Leave empty to first see what\'s dispatchable.',
                },
                {
                  value: "standby",
                  label: "I'm done for now",
                  description:
                    "Put Nexie on standby. The session stays open and reachable — message anytime to pick back up. Nothing is closed.",
                },
                {
                  value: "close",
                  label: "Close the session",
                  description:
                    "Explicitly archive this session. Standby is almost always what you want instead.",
                },
              ],
              required: true,
            },
            {
              id: "detail",
              kind: "free_text",
              label: "Detail (free-text, required for add_ticket / modify_ticket)",
              description:
                'Tell triage_board what to do. Examples: "all short-term", "feature_dev only", "abc123, def456", "close ticket abc12345", "create a sandbox-doctor refactor ticket". On dispatch_more: leave empty to see what\'s available before picking, or specify "all" / assignee / horizon / IDs to dispatch immediately.',
              placeholder:
                '"all", "feature_dev only", "abc123,def456" — or empty on dispatch_more to list first',
              rows: 3,
              required: false,
            },
          ],
          submitLabel: "Continue",
        },
      },
      // Deterministic projection of ask_continue.action → is_done.
      // Silent: the operator doesn't need to see it.
      derive_continue: { kind: "silent" },
      triage_board: {
        kind: "banner",
        label: "Updating the board",
        summaryField: "board_summary",
        // followCardKind: push a typed triage-summary card alongside
        // the banner so downstream human turns can read
        // created_issue_ids structurally — powers the
        // "Dispatch what I just created" radio option on ask_continue.
        followCardKind: "triageSummary",
      },
    },
  },
};

export function getFirstClassBot(id: string): FirstClassBot | null {
  return FIRST_CLASS_BOTS[id] ?? null;
}

export const DEFAULT_WHATS_NEXT_BOT_ID = "whats-next";
