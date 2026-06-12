import type { MarketplaceEntry } from "@/api/marketplace";

interface Props {
  entry: MarketplaceEntry;
  installing: boolean;
  onInstall: () => void;
  onOpen: () => void;
}

/** MarketplaceCard renders one entry as a compact tile. The card is
 *  clickable (opens the detail panel); the install button stops
 *  propagation so it doesn't double-trigger the open. Styled to match
 *  the catalog/launch surfaces' design tokens. */
export function MarketplaceCard({ entry, installing, onInstall, onOpen }: Props) {
  const label = entry.display_name?.trim() || entry.name;
  return (
    <li
      className="flex h-full flex-col gap-2 rounded border border-border-default bg-surface-2 p-3 transition-colors hover:bg-surface-3 focus-within:bg-surface-3"
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex flex-col items-start gap-1 text-left focus:outline-none"
      >
        <div className="flex w-full items-baseline justify-between gap-2">
          <span className="truncate text-sm font-medium text-fg-default">{label}</span>
          <span className="shrink-0 text-[10px] text-fg-subtle">
            {entry.installs} install{entry.installs === 1 ? "" : "s"}
          </span>
        </div>
        {entry.description && (
          <p className="line-clamp-2 text-xs text-fg-muted">{entry.description}</p>
        )}
        <div className="flex flex-wrap items-center gap-1 text-[10px] text-fg-subtle">
          {entry.author && <span>by {entry.author}</span>}
          {entry.version && <span className="font-mono">v{entry.version}</span>}
          {entry.presets && entry.presets.length > 0 && (
            <span>
              {entry.presets.length} preset{entry.presets.length === 1 ? "" : "s"}
            </span>
          )}
        </div>
        {entry.tags && entry.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {entry.tags.map((t) => (
              <span
                key={t}
                className="rounded bg-surface-1 px-1.5 py-0.5 text-[10px] text-fg-muted"
              >
                {t}
              </span>
            ))}
          </div>
        )}
      </button>
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-[10px] text-fg-subtle">
          {entry.repo_url}
          {entry.ref ? `#${entry.ref}` : ""}
        </span>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onInstall();
          }}
          disabled={installing}
          className="shrink-0 rounded bg-success/20 px-2.5 py-1 text-[11px] font-medium text-success hover:bg-success/30 disabled:opacity-50"
        >
          {installing ? "Installing…" : "Install"}
        </button>
      </div>
    </li>
  );
}
