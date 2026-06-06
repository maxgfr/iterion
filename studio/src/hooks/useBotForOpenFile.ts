import { useEffect, useMemo } from "react";

import type { BotEntryWithSchema } from "@/api/bots";
import { useBotsStore } from "@/store/bots";
import { useDocumentStore } from "@/store/document";

// matchesBundleMain reports whether the open file is `<bundle>/main.bot`
// for the given bot. It compares the bundle directory's basename so it
// tolerates the abs/relative mismatch between the registry's Entry.path
// (server-absolute) and the editor's currentFilePath (workspace-relative).
function matchesBundleMain(filePath: string, bot: BotEntryWithSchema): boolean {
  if (!bot.is_bundle) return false;
  const suffix = "/main.bot";
  if (!filePath.endsWith(suffix)) return false;
  const botDir = bot.path.replace(/\/+$/, "").split("/").pop() ?? "";
  const fileDir = filePath.slice(0, filePath.length - suffix.length).split("/").pop() ?? "";
  return botDir !== "" && botDir === fileDir;
}

/**
 * useBotForOpenFile resolves the bundle bot whose `main.bot` is the file
 * currently open in the editor, or null when the open file is not a
 * bundle's main.bot (loose .bot / .iter, or no file). Drives the
 * conditional Inspector "Bot" metadata tab. Triggers a one-time bots
 * fetch so the tab works on a cold editor load.
 */
export function useBotForOpenFile(): BotEntryWithSchema | null {
  const currentFilePath = useDocumentStore((s) => s.currentFilePath);
  const bots = useBotsStore((s) => s.bots);
  const fetchBots = useBotsStore((s) => s.fetch);

  useEffect(() => {
    if (bots === null) void fetchBots();
  }, [bots, fetchBots]);

  return useMemo(() => {
    if (!currentFilePath || !bots) return null;
    return bots.find((b) => matchesBundleMain(currentFilePath, b)) ?? null;
  }, [currentFilePath, bots]);
}
