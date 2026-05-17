// First-class bot registry — bots that get a dedicated /pilote
// experience instead of being launched generically through LaunchView.
//
// v0 hard-codes the single whats-next entry. When a second first-class
// bot lands, promote this registry to a manifest-driven discovery (or
// a server-side endpoint), and replace the const with a fetch.
//
// `nodeMap` describes how each node id of the workflow renders in the
// Pilote chat. The keys must match the `.bot` source — a rename there
// without updating the map silently drops the node from the chat.

export type PiloteNodeKind =
  | "banner"
  | "human"
  | "silent"
  | "issues-summary"
  | "roadmap";

export interface PiloteNodeMapEntry {
  kind: PiloteNodeKind;
  // Label shown in the progress banner ("Surveying repository…").
  label?: string;
  // For agent nodes whose output should be promoted to a typed card
  // after the banner closes. Currently used for `roadmap` and
  // `issuesSummary` follow-up renders.
  followCardKind?: "roadmap" | "issuesSummary";
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
  // Required for kind "human".
  textField?: string;
  // For "human" entries with approve/reject buttons: the schema field
  // name for the boolean outcome. human_review → "approved". Optional —
  // free-text-only turns leave this unset.
  approvedField?: string;
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
  nodeMap: Readonly<Record<string, PiloteNodeMapEntry>>;
}

export const FIRST_CLASS_BOTS: Readonly<Record<string, FirstClassBot>> = {
  "whats-next": {
    id: "whats-next",
    label: "What's Next",
    description:
      "Decide what to push on the board next. Iterion surveys the repo, drafts a roadmap, you approve, and it materialises as kanban issues the conductor can dispatch.",
    workflowPath: "examples/whats-next/main.bot",
    launcherVars: [
      { name: "workspace_dir", label: "Workspace", defaultFrom: "work_dir" },
    ],
    nodeMap: {
      explore: {
        kind: "banner",
        label: "Surveying the repository",
        summaryField: "summary",
      },
      ask_priorities: {
        kind: "human",
        prompt: "What matters right now?",
        textField: "context",
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
    },
  },
};

export function getFirstClassBot(id: string): FirstClassBot | null {
  return FIRST_CLASS_BOTS[id] ?? null;
}

export const DEFAULT_PILOTE_BOT_ID = "whats-next";
