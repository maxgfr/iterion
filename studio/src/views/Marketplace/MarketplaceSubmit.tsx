import { useId, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

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
  const urlId = useId();
  const refId = useId();
  const pathId = useId();
  const tagsId = useId();

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
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "Cancel" : "Submit…"}
        </Button>
      </div>
      {expanded && (
        <div className="mt-3 space-y-2">
          <div className="flex flex-col gap-1">
            <label
              htmlFor={urlId}
              className="text-caption uppercase tracking-wide text-fg-subtle"
            >
              Repository URL
            </label>
            <Input
              id={urlId}
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="git URL (https://… or git@…) or local path"
            />
          </div>
          <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
            <div className="flex flex-col gap-1">
              <label
                htmlFor={refId}
                className="text-caption uppercase tracking-wide text-fg-subtle"
              >
                Ref (optional)
              </label>
              <Input
                id={refId}
                type="text"
                value={ref}
                onChange={(e) => setRef(e.target.value)}
                placeholder="branch or tag"
              />
            </div>
            <div className="flex flex-col gap-1">
              <label
                htmlFor={pathId}
                className="text-caption uppercase tracking-wide text-fg-subtle"
              >
                Subpath / bot name (optional)
              </label>
              <Input
                id={pathId}
                type="text"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="e.g. bots/featurly"
              />
            </div>
          </div>
          <div className="flex flex-col gap-1">
            <label
              htmlFor={tagsId}
              className="text-caption uppercase tracking-wide text-fg-subtle"
            >
              Tags (comma-separated)
            </label>
            <Input
              id={tagsId}
              type="text"
              value={tagsRaw}
              onChange={(e) => setTagsRaw(e.target.value)}
              placeholder="review, kanban, sre"
            />
          </div>
          <div className="flex items-center justify-between gap-3">
            <p className="text-caption text-fg-subtle">
              The server clones the repo and validates the bundle (no install).
              Submitting again with the same name refreshes the entry.
            </p>
            <Button
              variant="success"
              size="sm"
              className="shrink-0"
              onClick={() => void handle()}
              disabled={submitting || !url.trim()}
              loading={submitting}
            >
              {submitting ? "Submitting…" : "Submit"}
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}
