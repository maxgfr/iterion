import { useState } from "react";

interface Props {
  onSubmit: (req: {
    repo_url: string;
    ref?: string;
    path?: string;
    tags?: string[];
  }) => Promise<void>;
}

/** MarketplaceSubmit is the inline form for adding a repository to the
 *  hosted registry. The actual cloning/inspection happens server-side
 *  (botinstall.Inspect); we surface the validation result via the
 *  parent's toast. Collapsed by default to keep the browse list above
 *  the fold. */
export function MarketplaceSubmit({ onSubmit }: Props) {
  const [expanded, setExpanded] = useState(false);
  const [url, setUrl] = useState("");
  const [ref, setRef] = useState("");
  const [path, setPath] = useState("");
  const [tagsRaw, setTagsRaw] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const reset = () => {
    setUrl("");
    setRef("");
    setPath("");
    setTagsRaw("");
  };

  const handle = async () => {
    const repo = url.trim();
    if (!repo) return;
    setSubmitting(true);
    try {
      const tags = tagsRaw
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean);
      await onSubmit({
        repo_url: repo,
        ref: ref.trim() || undefined,
        path: path.trim() || undefined,
        tags: tags.length > 0 ? tags : undefined,
      });
      reset();
      setExpanded(false);
    } catch {
      // toast already raised by the parent
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="rounded border border-border-default bg-surface-2 p-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xs font-medium text-fg-muted">Submit a repository</h2>
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="rounded bg-surface-1 px-2 py-1 text-micro text-fg-muted hover:bg-surface-3 hover:text-fg-default"
        >
          {expanded ? "Cancel" : "Submit…"}
        </button>
      </div>
      {expanded && (
        <div className="mt-3 space-y-2">
          <label className="flex flex-col gap-1">
            <span className="text-caption uppercase tracking-wide text-fg-subtle">Repository URL</span>
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="git URL (https://… or git@…) or local path"
              className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
            />
          </label>
          <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
            <label className="flex flex-col gap-1">
              <span className="text-caption uppercase tracking-wide text-fg-subtle">Ref (optional)</span>
              <input
                type="text"
                value={ref}
                onChange={(e) => setRef(e.target.value)}
                placeholder="branch or tag"
                className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-caption uppercase tracking-wide text-fg-subtle">Subpath / bot name (optional)</span>
              <input
                type="text"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="e.g. bots/featurly"
                className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
              />
            </label>
          </div>
          <label className="flex flex-col gap-1">
            <span className="text-caption uppercase tracking-wide text-fg-subtle">Tags (comma-separated)</span>
            <input
              type="text"
              value={tagsRaw}
              onChange={(e) => setTagsRaw(e.target.value)}
              placeholder="review, kanban, sre"
              className="w-full rounded border border-border-default bg-surface-1 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
            />
          </label>
          <div className="flex items-center justify-between gap-3">
            <p className="text-caption text-fg-subtle">
              The server clones the repo and validates the bundle (no install).
              Submitting again with the same name refreshes the entry.
            </p>
            <button
              type="button"
              onClick={() => void handle()}
              disabled={submitting || !url.trim()}
              className="shrink-0 rounded bg-success/20 px-2.5 py-1 text-micro font-medium text-success hover:bg-success/30 disabled:opacity-50"
            >
              {submitting ? "Submitting…" : "Submit"}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
