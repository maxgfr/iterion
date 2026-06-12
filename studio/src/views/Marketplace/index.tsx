import { useCallback, useEffect, useState } from "react";

import {
  installMarketplaceBot,
  listMarketplace,
  submitMarketplaceBot,
  type MarketplaceEntry,
} from "@/api/marketplace";
import { useUIStore } from "@/store/ui";

import { MarketplaceCard } from "./MarketplaceCard";
import { MarketplaceDetail } from "./MarketplaceDetail";
import { MarketplaceSubmit } from "./MarketplaceSubmit";

/** MarketplaceView is the hosted bot registry browse / submit / install
 *  surface. Mirrors the studio's other view conventions: page header,
 *  centred content, neutral surfaces, accent for primary actions. The
 *  view is gated by `serverInfo.marketplace_enabled` at the route level
 *  so it only mounts when the server has the registry store wired. */
export default function MarketplaceView() {
  const addToast = useUIStore((s) => s.addToast);
  const [entries, setEntries] = useState<MarketplaceEntry[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [tag, setTag] = useState("");
  const [activeSlug, setActiveSlug] = useState<string | null>(null);
  const [installing, setInstalling] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const list = await listMarketplace(search, tag);
      setEntries(list);
    } catch (e) {
      addToast(e instanceof Error ? e.message : String(e), "error");
    } finally {
      setLoading(false);
    }
  }, [search, tag, addToast]);

  // Debounced refetch on search/tag changes so typing in the search box
  // doesn't fire a request per keystroke.
  useEffect(() => {
    const t = window.setTimeout(() => void refresh(), 200);
    return () => window.clearTimeout(t);
  }, [refresh]);

  const onInstall = async (e: MarketplaceEntry) => {
    setInstalling(e.slug);
    try {
      const res = await installMarketplaceBot(e.slug);
      addToast(
        `Installed ${res.install.name} → ${res.install.installed_path}`,
        "success",
      );
      // Reflect the bumped install counter without a full refetch.
      setEntries((prev) =>
        prev?.map((x) => (x.slug === e.slug ? res.entry : x)) ?? prev,
      );
    } catch (err) {
      addToast(err instanceof Error ? err.message : String(err), "error");
    } finally {
      setInstalling(null);
    }
  };

  const onSubmit = async (req: {
    repo_url: string;
    ref?: string;
    path?: string;
    tags?: string[];
  }) => {
    try {
      const stored = await submitMarketplaceBot(req);
      addToast(`Added "${stored.display_name || stored.name}" to the marketplace`, "success");
      await refresh();
      setActiveSlug(stored.slug);
    } catch (e) {
      addToast(e instanceof Error ? e.message : String(e), "error");
      throw e;
    }
  };

  const active = entries?.find((e) => e.slug === activeSlug) ?? null;

  return (
    <div className="flex h-full min-h-0 flex-col bg-surface-1 text-fg-default">
      <header className="border-b border-border-default px-6 py-4">
        <div className="mx-auto flex max-w-5xl flex-col gap-1">
          <h1 className="text-base font-semibold text-fg-default">Bot marketplace</h1>
          <p className="text-xs text-fg-muted">
            Browse the hosted registry, submit a repository, or install a
            published bot into this workspace's <code className="text-fg-default">.botz/</code>.
          </p>
        </div>
      </header>

      <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-4 overflow-y-auto px-6 py-4">
        <section className="flex flex-col gap-2">
          <div className="flex flex-wrap items-end gap-2">
            <label className="flex min-w-[14rem] flex-1 flex-col gap-1">
              <span className="text-[10px] uppercase tracking-wide text-fg-subtle">Search</span>
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="name, description, tag…"
                className="rounded border border-border-default bg-surface-2 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
              />
            </label>
            <label className="flex w-44 flex-col gap-1">
              <span className="text-[10px] uppercase tracking-wide text-fg-subtle">Filter by tag</span>
              <input
                type="text"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
                placeholder="(e.g. review)"
                className="rounded border border-border-default bg-surface-2 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
              />
            </label>
            <button
              type="button"
              onClick={() => void refresh()}
              className="rounded bg-surface-2 px-2.5 py-1.5 text-xs text-fg-muted hover:bg-surface-3 hover:text-fg-default"
            >
              Refresh
            </button>
          </div>
        </section>

        <MarketplaceSubmit onSubmit={onSubmit} />

        <section className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <h2 className="text-xs font-medium text-fg-muted">
              {entries === null
                ? "Loading bots…"
                : entries.length === 0
                  ? "No bots in the marketplace yet"
                  : `${entries.length} bot${entries.length === 1 ? "" : "s"}`}
            </h2>
            {loading && entries && entries.length > 0 && (
              <span className="text-[10px] text-fg-subtle">Refreshing…</span>
            )}
          </div>
          {entries === null ? null : entries.length === 0 ? (
            <div className="rounded border border-border-default bg-surface-2 p-4 text-xs text-fg-muted">
              Use the form above to submit a repository. Submission validates the
              bundle and indexes its metadata; nothing is installed until you
              click <span className="text-fg-default">Install</span> on its card.
            </div>
          ) : (
            <ul className="grid grid-cols-1 gap-2 md:grid-cols-2">
              {entries.map((e) => (
                <MarketplaceCard
                  key={e.slug}
                  entry={e}
                  installing={installing === e.slug}
                  onInstall={() => void onInstall(e)}
                  onOpen={() => setActiveSlug(e.slug)}
                />
              ))}
            </ul>
          )}
        </section>
      </div>

      {active && (
        <MarketplaceDetail
          entry={active}
          installing={installing === active.slug}
          onInstall={() => void onInstall(active)}
          onClose={() => setActiveSlug(null)}
        />
      )}
    </div>
  );
}
