import { useCallback, useEffect, useState } from "react";
import { useLocation } from "wouter";
import {
  FilePlusIcon,
  FileIcon,
  TrashIcon,
  ChevronDownIcon,
  ChevronRightIcon,
} from "@radix-ui/react-icons";

import * as api from "@/api/client";
import { getOrCreateDocumentStore } from "@/store/document";
import { openExampleIntoStore } from "@/lib/openExample";
import { useRecentsStore } from "@/store/recents";
import { useTabsStore } from "@/store/tabs";
import { useUIStore } from "@/store/ui";
import { useConfirm } from "@/hooks/useConfirm";
import { basename } from "@/lib/format";
import { botIdentity } from "@/lib/personas";
import { EmptyState } from "@/components/ui/EmptyState";
import { BotCatalogDialog } from "@/components/Catalog/BotCatalogDialog";

// First-class bots are static for the lifetime of the server process, so
// cache the first successful response and reuse it on every subsequent
// mount. Avoids a network round-trip every time the user lands on /.
let examplesCache: api.ExampleEntry[] | null = null;
let examplesPromise: Promise<api.ExampleEntry[]> | null = null;

function loadExamples(): Promise<api.ExampleEntry[]> {
  if (examplesCache) return Promise.resolve(examplesCache);
  if (!examplesPromise) {
    examplesPromise = api.listExampleEntries().then((list) => {
      examplesCache = list;
      return list;
    }).catch((err) => {
      examplesPromise = null;
      throw err;
    });
  }
  return examplesPromise;
}

type Variant = "card" | "plain";

interface Props {
  // "card" (default): bordered section with header, used on Home.
  // "plain": no chrome, used as the EditorTabsView empty state where
  // the host already provides a centered container.
  variant?: Variant;
}

// RecentFilesPanel (a.k.a. workflow picker) shows the three ways to
// start an editor tab: a fresh blank workflow, one of the bundled
// examples, or one of the user's recent files. Every entry point
// opens (or focuses) an editor tab via the tabs store — never the
// singleton document store — so multi-file editing stays consistent.
export default function RecentFilesPanel({ variant = "card" }: Props) {
  const [, setLocation] = useLocation();
  const recents = useRecentsStore((s) => s.recents);
  const pushRecent = useRecentsStore((s) => s.pushRecent);
  const removeRecent = useRecentsStore((s) => s.removeRecent);
  const clearRecents = useRecentsStore((s) => s.clearRecents);
  const addToast = useUIStore((s) => s.addToast);

  const [examples, setExamples] = useState<api.ExampleEntry[]>([]);
  const [examplesOpen, setExamplesOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [catalogOpen, setCatalogOpen] = useState(false);
  const { confirm, dialog: confirmDialog } = useConfirm();

  useEffect(() => {
    let cancelled = false;
    loadExamples()
      .then((list) => {
        if (!cancelled) setExamples(list);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (recents.length === 0 && examples.length > 0) setExamplesOpen(true);
  }, [recents.length, examples.length]);

  const handleNewBlank = useCallback(() => {
    useTabsStore.getState().newEditorTab();
    setLocation("/editor");
  }, [setLocation]);

  const handleOpenRecent = useCallback(
    (path: string) => {
      // Opening a recent file routes through openTab — EditorTabHost
      // will fetch the document via api.openFile on mount and bind it
      // to the per-tab store. Re-clicking the same path focuses the
      // existing tab instead of reloading.
      useTabsStore.getState().openTab("editor", { file: path });
      pushRecent(path);
      setLocation(`/editor?file=${encodeURIComponent(path)}`);
    },
    [pushRecent, setLocation],
  );

  const handleOpenExample = useCallback(
    async (name: string) => {
      setBusy(true);
      // Open the bot in a fresh editor tab. Create the tab first so its
      // per-tab document store exists, then load + apply via the shared
      // helper (binds bots/<name> so Run enables, keeps source/diagnostics,
      // marks the loaded state as the clean baseline). On failure, close
      // the empty tab so a load error doesn't strand an untitled tab.
      const tabId = useTabsStore.getState().newEditorTab(name);
      try {
        await openExampleIntoStore(name, getOrCreateDocumentStore(tabId).getState());
        setLocation("/editor");
      } catch {
        useTabsStore.getState().closeTab(tabId);
        addToast("Failed to open example", "error");
      } finally {
        setBusy(false);
      }
    },
    [addToast, setLocation],
  );

  const body = (
    <div className={variant === "card" ? "p-3 space-y-3" : "space-y-3"}>
      <button
        onClick={handleNewBlank}
        disabled={busy}
        className="w-full flex items-center gap-2 px-3 py-2 rounded-md bg-accent-soft hover:bg-accent/20 border border-accent/40 text-sm disabled:opacity-50"
      >
        <FilePlusIcon className="w-4 h-4" />
        <span className="font-medium">New blank workflow</span>
      </button>

      {examples.length > 0 && (
        <div>
          <div className="flex items-center justify-between px-1">
            <button
              onClick={() => setExamplesOpen((v) => !v)}
              className="flex items-center gap-1 text-xs font-medium text-fg-muted hover:text-fg-default"
            >
              {examplesOpen ? (
                <ChevronDownIcon className="w-3 h-3" />
              ) : (
                <ChevronRightIcon className="w-3 h-3" />
              )}
              <span>Bots ({examples.length})</span>
            </button>
            <button
              onClick={() => setCatalogOpen(true)}
              className="text-[10px] text-fg-subtle hover:text-fg-default"
              title="Enable/disable bots and edit their metadata"
            >
              Manage
            </button>
          </div>
          {examplesOpen && (
            <ul className="mt-1 space-y-0.5">
              {examples.map((ex) => {
                // Persona (manifest display_name) + emoji on line 1, with
                // the technical name (the first path segment, e.g.
                // "whats-next") muted beside it and a one-line description
                // below. The technical id also drives the emoji/colour
                // lookup. Falls back to the raw name for an embedded
                // recipe with no on-disk persona.
                const techName = ex.name.split("/")[0];
                const identity = botIdentity(techName);
                return (
                  <li key={ex.name}>
                    <button
                      onClick={() => handleOpenExample(ex.name)}
                      disabled={busy}
                      className="w-full flex items-start gap-2 px-2 py-1.5 rounded hover:bg-surface-2 text-left disabled:opacity-50"
                    >
                      <span
                        className="text-sm leading-none shrink-0 mt-0.5"
                        aria-hidden="true"
                      >
                        {identity.emoji}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex items-baseline gap-1.5">
                          <span
                            className={`truncate font-medium text-xs ${identity.color}`}
                          >
                            {ex.display_name || ex.name}
                          </span>
                          {ex.display_name && (
                            <span className="font-mono text-fg-subtle text-[10px] truncate shrink-0">
                              {techName}
                            </span>
                          )}
                        </span>
                        {ex.description && (
                          <span className="block truncate text-fg-muted text-[11px] mt-0.5">
                            {ex.description}
                          </span>
                        )}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}

      <div>
        <div className="flex items-center justify-between px-1">
          <span className="text-xs font-medium text-fg-muted">
            Recent ({recents.length})
          </span>
          {recents.length > 0 && (
            <button
              onClick={async () => {
                const ok = await confirm({
                  title: "Clear recent files?",
                  message:
                    "All entries in the Recent list will be removed. Your files on disk are not affected.",
                  confirmLabel: "Clear",
                  confirmVariant: "danger",
                });
                if (ok) clearRecents();
              }}
              className="text-[10px] text-fg-subtle hover:text-fg-default"
            >
              Clear all
            </button>
          )}
        </div>
        {recents.length === 0 ? (
          <EmptyState
            className="mt-2 py-3"
            message={
              examples.length > 0
                ? "No recent files yet — start from an example above or create a new workflow."
                : "No recent files yet — create a new workflow."
            }
          />
        ) : (
          <ul className="mt-1 space-y-0.5">
            {recents.map((path) => (
              <li key={path} className="group flex items-center gap-1">
                <button
                  onClick={() => handleOpenRecent(path)}
                  disabled={busy}
                  className="flex-1 min-w-0 flex items-center gap-2 px-2 py-1.5 rounded hover:bg-surface-2 text-left text-xs disabled:opacity-50"
                  title={path}
                >
                  <FileIcon className="w-3.5 h-3.5 text-fg-subtle shrink-0" />
                  <span className="font-medium truncate">
                    {basename(path)}
                  </span>
                  {basename(path) !== path && (
                    <span className="text-fg-subtle text-[10px] truncate">
                      {path}
                    </span>
                  )}
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    removeRecent(path);
                  }}
                  className="p-1 text-fg-subtle hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity"
                  title="Remove from recents"
                  aria-label={`Remove ${path} from recents`}
                >
                  <TrashIcon className="w-3.5 h-3.5" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      {confirmDialog}
      <BotCatalogDialog open={catalogOpen} onOpenChange={setCatalogOpen} />
    </div>
  );

  if (variant === "plain") return body;

  return (
    <section className="flex flex-col bg-surface-1 border border-border-default rounded-lg overflow-hidden">
      <header className="px-4 py-2.5 border-b border-border-default flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">
          Workflows
        </h2>
        <button
          onClick={() => setCatalogOpen(true)}
          className="text-[10px] text-fg-subtle hover:text-fg-default"
          title="Enable/disable bots and edit their metadata"
        >
          Manage bots
        </button>
      </header>
      {body}
    </section>
  );
}
