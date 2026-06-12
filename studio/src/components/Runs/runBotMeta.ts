import type { RunSummary } from "@/api/runs";
import { botIdentity } from "@/lib/personas";

import { workflowLabel } from "./runListSortGroup";

// A bot present in the runs list — one chip on the "by bot" filter strip.
// `key` is the stable filter value: it IS workflowLabel (bundle_name ||
// workflow_name), so the bot filter, the "Workflow" sort, and
// group-by-workflow all sit on the same identity — runBotMeta owns only
// the *presentation* (label + emoji + count). `label` is the readable
// persona name; `emoji` its avatar glyph; `count` how many runs match.
export interface BotDescriptor {
  key: string;
  label: string;
  emoji: string;
  count: number;
}

// botLabel is the human-facing name: the manifest persona when present,
// else the technical bundle name, else the workflow name. Mirrors the
// fallback chain BotPicker uses.
export function botLabel(run: RunSummary): string {
  return (
    run.bundle_display_name?.trim() ||
    run.bundle_name?.trim() ||
    run.workflow_name?.trim() ||
    "(unnamed)"
  );
}

// botEmoji resolves the avatar glyph from the run's technical bot id
// (bundle_name, falling back to workflow_name for plain runs). Unknown
// bots get the deterministic 🤖 fallback from botIdentity.
export function botEmoji(run: RunSummary): string {
  return botIdentity(run.bundle_name || run.workflow_name).emoji;
}

// availableBots returns the distinct bots present in the fetched list,
// with per-bot counts, sorted by label (case-insensitive). Mirrors
// availableRepos: the chip strip uses it so only bots that could produce
// a hit are shown, and the count comes from the same single pass. The
// first run seen for each key supplies the label + emoji.
export function availableBots(runs: RunSummary[]): BotDescriptor[] {
  const byKey = new Map<string, BotDescriptor>();
  for (const run of runs) {
    const key = workflowLabel(run);
    if (!key) continue;
    const existing = byKey.get(key);
    if (existing) {
      existing.count++;
      continue;
    }
    byKey.set(key, { key, label: botLabel(run), emoji: botEmoji(run), count: 1 });
  }
  return Array.from(byKey.values()).sort((a, b) =>
    a.label.localeCompare(b.label, undefined, { sensitivity: "base" }),
  );
}
