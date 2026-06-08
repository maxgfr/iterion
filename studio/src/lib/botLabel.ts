import type { BotEntryWithSchema } from "@/api/bots";
import { basename } from "@/lib/format";

/**
 * A human label for a workspace path, used for the editor tab + document
 * title. A bundle's main file is non-distinctive — every bot's is
 * `main.bot` — so for such a path show the bot's persona display_name
 * (e.g. "Featurly"), falling back to its technical id (the parent dir
 * segment, e.g. "feature-dev") when the catalog isn't loaded or the bot
 * ships no manifest persona. Loose files keep their meaningful basename.
 */
export function botDisplayLabel(
  path: string,
  bots: BotEntryWithSchema[] | null,
): string {
  const segs = path.split(/[\\/]/).filter(Boolean);
  const base = segs[segs.length - 1] ?? "";
  if ((base === "main.bot" || base === "main.iter") && segs.length >= 2) {
    const tech = segs[segs.length - 2]!;
    const bot = bots?.find((b) => {
      const relTech = (b.rel_path ?? "").split(/[\\/]/).filter(Boolean).pop();
      return relTech === tech || b.name === tech;
    });
    return bot?.display_name || tech;
  }
  return basename(path);
}
