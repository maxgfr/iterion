import { useEffect, useState } from "react";
import { useLocation } from "wouter";

import type { BotEntryWithSchema } from "@/api/bots";
import { Dialog } from "@/components/ui";
import { botIdentity } from "@/lib/personas";
import { useBotsStore } from "@/store/bots";
import { useTabsStore } from "@/store/tabs";
import { useUIStore } from "@/store/ui";

// bundleMainRel returns the workspace-relative main.bot path the editor's
// openFile expects, using the server-provided rel_path. Null when the bot
// isn't an editable bundle or the server couldn't relativise it.
function bundleMainRel(b: BotEntryWithSchema): string | null {
  return b.is_bundle && b.rel_path ? `${b.rel_path}/main.bot` : null;
}

/**
 * BotCatalogDialog is the catalog manager: every discovered bot with an
 * instant enable/disable toggle (workspace overlay — no manifest/git
 * churn) and a jump-to-edit affordance that opens the bot's main.bot in
 * the editor with the Bot metadata tab focused. Disabled bots are shown
 * desaturated so they can be flipped back on.
 */
export function BotCatalogDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [, setLocation] = useLocation();
  const bots = useBotsStore((s) => s.bots);
  const loading = useBotsStore((s) => s.loading);
  const fetchBots = useBotsStore((s) => s.fetch);
  const refetch = useBotsStore((s) => s.refetch);
  const setOverlay = useBotsStore((s) => s.setOverlay);
  const addToast = useUIStore((s) => s.addToast);
  const setActiveTab = useUIStore((s) => s.setActiveTab);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (bots === null) void fetchBots();
    else void refetch();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onToggle = async (b: BotEntryWithSchema) => {
    const next = b.enabled === false; // flipping to this state
    const label = b.display_name?.trim() || b.name;
    setBusy(b.name);
    try {
      await setOverlay(b.name, next);
      addToast(
        next ? `${label} enabled — exposed to Nexie` : `${label} disabled — hidden from Nexie`,
        next ? "success" : "info",
      );
    } catch (e) {
      addToast(e instanceof Error ? e.message : `Failed to update ${label}`, "error");
    } finally {
      setBusy(null);
    }
  };

  const onEdit = (b: BotEntryWithSchema) => {
    const rel = bundleMainRel(b);
    if (!rel) return;
    useTabsStore.getState().openTab("editor", { file: rel });
    setActiveTab("bot");
    onOpenChange(false);
    setLocation(`/editor?file=${encodeURIComponent(rel)}`);
  };

  const rows = bots ?? [];

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Manage bots"
      description="Toggle which bots are exposed to Nexie and the board picker. Toggles are workspace-local (they don't edit the bot's manifest)."
      widthClass="max-w-2xl"
    >
      <div className="max-h-[60vh] overflow-y-auto">
        {loading && rows.length === 0 && (
          <p className="px-1 py-4 text-sm text-fg-subtle">Loading bots…</p>
        )}
        {!loading && rows.length === 0 && (
          <p className="px-1 py-4 text-sm text-fg-subtle">No bots discovered in this workspace.</p>
        )}
        <ul className="space-y-0.5">
          {rows.map((b) => {
            const enabled = b.enabled !== false;
            const identity = botIdentity(b.name);
            const label = b.display_name?.trim();
            const canEdit = bundleMainRel(b) !== null;
            return (
              <li
                key={b.name}
                className={`flex items-center gap-3 rounded px-2 py-2 hover:bg-surface-2 ${enabled ? "" : "opacity-60"}`}
              >
                <span className="shrink-0 text-base leading-none" aria-hidden="true">
                  {identity.emoji}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline gap-1.5">
                    <span className={`truncate text-sm font-medium ${identity.color}`}>
                      {label || b.name}
                    </span>
                    {label && (
                      <span className="shrink-0 truncate font-mono text-[10px] text-fg-subtle">
                        {b.name}
                      </span>
                    )}
                    {!b.is_bundle && (
                      <span className="shrink-0 text-[10px] text-fg-subtle">(single file)</span>
                    )}
                  </div>
                  {b.description && (
                    <div className="truncate text-[11px] text-fg-muted">{b.description}</div>
                  )}
                </div>
                {canEdit && (
                  <button
                    type="button"
                    onClick={() => onEdit(b)}
                    className="shrink-0 rounded bg-surface-2 px-2 py-1 text-[11px] text-fg-muted hover:bg-surface-3 hover:text-fg-default"
                  >
                    Edit
                  </button>
                )}
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  disabled={busy === b.name}
                  onClick={() => onToggle(b)}
                  title={enabled ? "Disable (hide from Nexie)" : "Enable (expose to Nexie)"}
                  className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors disabled:opacity-50 ${
                    enabled
                      ? "bg-success/20 text-success hover:bg-success/30"
                      : "bg-surface-2 text-fg-subtle hover:bg-surface-3"
                  }`}
                >
                  {enabled ? "On" : "Off"}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
    </Dialog>
  );
}
