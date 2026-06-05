/**
 * Visual identity for the iterion bot team — the persona name, an emoji
 * avatar, and an accent colour each bot is rendered with across the
 * studio (run-header chip, board bot picker, chat bubbles).
 *
 * The authoritative *name* is the bundle's `manifest.yaml` `display_name`
 * (carried on the run as `bundle_display_name` and on the registry Entry
 * as `display_name`). This map supplies the matching emoji + colour
 * (presentation, not data) and a fallback persona name for the few
 * places that have no API/run data to read (e.g. the WhatsNext chat
 * bubble). Keys are the bot's CANONICAL technical id (kebab); lookups
 * normalise snake_case → kebab so `feature_dev` and `feature-dev` both
 * resolve.
 */

export interface BotIdentity {
  /** Operator-facing persona name. Fallback only — prefer the run /
   *  registry manifest `display_name` when present. */
  persona: string;
  /** Single emoji standing in for the bot's avatar. */
  emoji: string;
  /** Tailwind text-colour class for the persona name + emoji. */
  color: string;
}

export const BOT_IDENTITY: Record<string, BotIdentity> = {
  "whats-next": { persona: "Nexie", emoji: "🧭", color: "text-sky-400" },
  "feature-dev": { persona: "Featurly", emoji: "🛠️", color: "text-emerald-400" },
  "branch-improve-loop": { persona: "Billy", emoji: "🌿", color: "text-violet-400" },
  "whole-improve-loop": { persona: "Willy", emoji: "🌍", color: "text-teal-400" },
  "doc-align": { persona: "Doki", emoji: "📚", color: "text-amber-400" },
  "code-review": { persona: "Revi", emoji: "🔎", color: "text-cyan-400" },
  "sec-audit-source": { persona: "Seki", emoji: "🛡️", color: "text-rose-400" },
  "sec-audit-deps": { persona: "Depsy", emoji: "📦", color: "text-orange-400" },
  "secured-renovacy": { persona: "Renovacy", emoji: "⬆️", color: "text-lime-400" },
};

const FALLBACK_COLORS = [
  "text-sky-400",
  "text-emerald-400",
  "text-violet-400",
  "text-amber-400",
  "text-rose-400",
  "text-cyan-400",
  "text-orange-400",
  "text-lime-400",
];

/** Canonicalise a technical bot id: lower-case, collapse runs of `_`/`-`
 *  into a single `-`. So `feature_dev`, `feature-dev` and `Feature_Dev`
 *  all map to the same key. */
function canon(name: string): string {
  return name.trim().toLowerCase().replace(/[_-]+/g, "-");
}

/**
 * botIdentity resolves a bot's visual identity from its technical id
 * (kebab or snake). Unknown bots get a deterministic fallback — a stable
 * colour derived from the name hash plus a generic robot emoji — so the
 * UI never renders a bare chip. `persona` is a fallback name; when the
 * run / registry carries a manifest `display_name`, prefer that.
 */
export function botIdentity(name: string | undefined | null): BotIdentity {
  const key = canon(name ?? "");
  const hit = BOT_IDENTITY[key];
  if (hit) return hit;
  let h = 0;
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return {
    persona: name?.trim() || "bot",
    emoji: "🤖",
    color: FALLBACK_COLORS[h % FALLBACK_COLORS.length] ?? "text-sky-400",
  };
}

/**
 * BOT_PERSONAS — bare technical-id → persona-name map, derived from
 * BOT_IDENTITY. Kept for the WhatsNext chat bubble, which has no run
 * data to read a manifest display_name from. New code should call
 * botIdentity(name).persona (or read the run's bundle_display_name).
 */
export const BOT_PERSONAS: Record<string, string> = Object.fromEntries(
  Object.entries(BOT_IDENTITY).map(([k, v]) => [k, v.persona]),
);
