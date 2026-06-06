/**
 * Visual identity for the iterion bot team — an emoji avatar + accent
 * colour each bot is rendered with across the studio (run-header chip,
 * board bot picker, chat bubbles).
 *
 * PRESENTATION ONLY. The bot's *name* is the manifest `display_name`
 * (carried on the run as `bundle_display_name` and on the registry Entry
 * as `display_name`); this map never holds the authoritative name, only
 * the emoji + colour that go with it. Keys are the bot's CANONICAL
 * technical id (kebab); lookups normalise snake_case → kebab so
 * `feature_dev` and `feature-dev` both resolve.
 */

export interface BotIdentity {
  /** Single emoji standing in for the bot's avatar. */
  emoji: string;
  /** Tailwind text-colour class for the persona name + emoji. */
  color: string;
}

export const BOT_IDENTITY: Record<string, BotIdentity> = {
  "whats-next": { emoji: "🧭", color: "text-sky-400" },
  "feature-dev": { emoji: "🛠️", color: "text-emerald-400" },
  "branch-improve-loop": { emoji: "🌿", color: "text-violet-400" },
  "whole-improve-loop": { emoji: "🌍", color: "text-teal-400" },
  "docs-refresh": { emoji: "📚", color: "text-amber-400" },
  "doc-align": { emoji: "📚", color: "text-amber-400" }, // back-compat alias (renamed 2026-06); keeps Doki's identity on pre-rename runs
  "code-review": { emoji: "🔎", color: "text-cyan-400" },
  "sec-audit-source": { emoji: "🛡️", color: "text-rose-400" },
  "sec-audit-deps": { emoji: "📦", color: "text-orange-400" },
  "secured-renovacy": { emoji: "⬆️", color: "text-lime-400" },
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
 * botIdentity resolves a bot's emoji + accent colour from its technical id
 * (kebab or snake). Unknown bots get a deterministic fallback — a stable
 * colour derived from the name hash plus a generic robot emoji — so every
 * bot gets a distinct chip and the UI never renders a bare one. The NAME
 * is not this map's concern: read the manifest display_name (a run's
 * bundle_display_name, or the registry entry's display_name).
 */
export function botIdentity(name: string | undefined | null): BotIdentity {
  const key = canon(name ?? "");
  const hit = BOT_IDENTITY[key];
  if (hit) return hit;
  let h = 0;
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return {
    emoji: "🤖",
    color: FALLBACK_COLORS[h % FALLBACK_COLORS.length] ?? "text-sky-400",
  };
}
