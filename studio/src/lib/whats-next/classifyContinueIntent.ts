// classifyContinueIntent maps a free-text "what's next" instruction
// into the structured {action, detail} pair the ask_continue turn
// expects. Used by WhatsNext Quick mode: the operator types one line
// instead of picking a radio + filling a detail box.
//
// This is a deliberate HEURISTIC, not an LLM call — it runs instantly,
// costs nothing, and its output is always shown to the operator in a
// dry-run banner before anything executes. Mis-classifications are
// cheap: the operator edits the action/detail in the banner or flips
// Quick mode off. A confidence score drives how prominently the
// banner asks for confirmation (low confidence → rationale shown by
// default). An LLM-backed classifier is a possible v2 upgrade behind
// the same interface.

export type ContinueAction =
  | "add_ticket"
  | "modify_ticket"
  | "dispatch_more"
  | "standby"
  | "close";

// Session-control actions carry no `detail` — they pause/close the
// session rather than mutating the board. Shared by the classifier
// and the Quick-mode footer so the "no detail" rule stays in one place.
export function isSessionControlAction(action: ContinueAction): boolean {
  return action === "standby" || action === "close";
}

export interface ClassifiedIntent {
  action: ContinueAction;
  detail: string;
  // 0..1 — how confident the heuristic is. Drives the dry-run banner's
  // emphasis; never gates execution (the operator always confirms).
  confidence: number;
  // Short human-readable reason for the classification, surfaced when
  // confidence is low so the operator understands the guess.
  rationale: string;
}

interface Rule {
  action: ContinueAction;
  // Matches anywhere in the lowercased text.
  patterns: RegExp[];
  // When set, the captured group 1 becomes the detail; otherwise the
  // full original text is the detail.
  detailFrom?: RegExp;
  rationale: string;
}

// Ordered by specificity — the first matching rule wins. The two
// session-control intents are checked first: "close" (explicit
// archive — most specific) then "standby" ("I'm done for now", keeps
// the session reachable). Then the board-mutation verbs, then dispatch.
const RULES: Rule[] = [
  {
    action: "close",
    patterns: [
      /\b(close|end|archive)\s+(the\s+|this\s+)?(session|chat|conversation|nexie)\b/i,
      /\b(shut\s?down|terminate|kill\s+nexie|arr[êe]te\s+nexie|ferme\s+(la\s+)?session)\b/i,
    ],
    rationale: "phrasing signals explicitly closing/archiving the session",
  },
  {
    action: "standby",
    patterns: [
      /^\s*(done|stop|finish|finished|that'?s all|c'?est (tout|bon|fini)|fini|termin[ée]|exit|quit|end|later|pause|standby|brb)\b/i,
      /\b(i'?m done|done for now|nothing else|rien d'?autre|plus tard)\b/i,
    ],
    rationale: "phrasing signals pausing for now — the session stays open",
  },
  {
    action: "dispatch_more",
    patterns: [
      /\b(dispatch|ship|promote|push (to )?ready|envoie|lance|run)\b/i,
    ],
    detailFrom: /\b(?:dispatch|ship|promote|push(?: to)? ready|envoie|lance|run)\b\s*(.*)$/i,
    rationale: "verb implies pushing tickets to ready",
  },
  {
    action: "add_ticket",
    patterns: [
      /\b(add|create|new ticket|new issue|open (a )?(ticket|issue)|cr[ée]e|ajoute|nouveau ticket)\b/i,
    ],
    detailFrom:
      /\b(?:add(?: a)?(?: ticket| issue)?|create(?: a)?(?: ticket| issue)?|new ticket|new issue|open(?: a)?(?: ticket| issue)?|cr[ée]e(?:r)?|ajoute(?:r)?|nouveau ticket)\b\s*(?:to\s+|for\s+|:\s*)?(.*)$/i,
    rationale: "verb implies creating a new ticket",
  },
  {
    action: "modify_ticket",
    patterns: [
      /\b(close|move|reassign|re-?assign|assign|label|rename|update|edit|change|ferme|d[ée]place|assigne|renomme|modifie)\b/i,
    ],
    rationale: "verb implies modifying an existing ticket",
  },
];

export function classifyContinueIntent(text: string): ClassifiedIntent {
  const trimmed = text.trim();
  if (trimmed === "") {
    return {
      action: "modify_ticket",
      detail: "",
      confidence: 0,
      rationale: "empty input — defaulting to modify; please pick an action",
    };
  }

  for (const rule of RULES) {
    if (!rule.patterns.some((p) => p.test(trimmed))) continue;
    let detail = trimmed;
    if (rule.detailFrom) {
      const m = trimmed.match(rule.detailFrom);
      if (m && typeof m[1] === "string" && m[1].trim() !== "") {
        detail = m[1].trim();
      }
    }
    // Session-control intents carry no meaningful detail.
    if (isSessionControlAction(rule.action)) detail = "";
    return {
      action: rule.action,
      detail,
      confidence: 0.8,
      rationale: rule.rationale,
    };
  }

  // No rule matched — fall back to modify_ticket (the most general
  // board action) with the full text as detail, but flag low
  // confidence so the dry-run banner shows the rationale and nudges
  // the operator to verify.
  return {
    action: "modify_ticket",
    detail: trimmed,
    confidence: 0.3,
    rationale: "no clear action verb — guessing modify; verify or edit below",
  };
}
