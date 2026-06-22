import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";

import type { BotEntryWithSchema } from "@/api/bots";
import {
  type ForgeConnection,
  type ForgeEnablePreview,
  type ForgeRepo,
  enableForgeRepoBots,
  listForgeRepos,
  previewForgeEnable,
} from "@/api/forgeConnections";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import {
  GROUP_LABELS,
  GROUP_ORDER,
  primaryGroup,
  triggerChips,
} from "@/lib/triggers";

export function EnableRepoPanel({
  teamID,
  conn,
  repoBots,
  preselectBot,
  onDone,
  onCancel,
  onError,
}: {
  teamID: string;
  conn: ForgeConnection;
  repoBots: BotEntryWithSchema[];
  preselectBot?: string;
  onDone: () => void;
  onCancel: () => void;
  onError: (m: string) => void;
}) {
  const [search, setSearch] = useState("");
  const [repos, setRepos] = useState<ForgeRepo[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [repo, setRepo] = useState("");
  const [selectedBots, setSelectedBots] = useState<string[]>(
    preselectBot ? [preselectBot] : [],
  );
  const [preview, setPreview] = useState<ForgeEnablePreview | null>(null);
  const [busy, setBusy] = useState(false);

  const loadRepos = async () => {
    setLoadingRepos(true);
    try {
      setRepos(await listForgeRepos(teamID, conn.id, search));
    } catch (e) {
      onError(errorMessage(e));
    } finally {
      setLoadingRepos(false);
    }
  };

  useEffect(() => {
    void loadRepos();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fetch the authoritative preview (native events the hook will subscribe
  // to + identity + any scope/forge-block conflicts) whenever the selection
  // changes, so the operator sees exactly what Enable will provision.
  useEffect(() => {
    if (!repo || selectedBots.length === 0) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    void previewForgeEnable(teamID, conn.id, repo, selectedBots)
      .then((p) => {
        if (!cancelled) setPreview(p);
      })
      .catch(() => {
        if (!cancelled) setPreview(null);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo, selectedBots]);

  const toggleBot = (name: string) =>
    setSelectedBots((s) => (s.includes(name) ? s.filter((b) => b !== name) : [...s, name]));

  const hasConflicts = (preview?.conflicts?.length ?? 0) > 0;

  const enable = async () => {
    if (!repo || selectedBots.length === 0) return;
    setBusy(true);
    try {
      await enableForgeRepoBots(teamID, conn.id, repo, selectedBots);
      onDone();
    } catch (e) {
      onError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="bg-surface-0 border border-border-subtle rounded p-3 space-y-3">
      <div className="flex gap-2 items-center">
        <div className="flex-1">
          <label htmlFor="forge-repo-search" className="sr-only">
            Search repos
          </label>
          <Input
            id="forge-repo-search"
            placeholder="Search repos…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void loadRepos();
            }}
          />
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => void loadRepos()}
          loading={loadingRepos}
        >
          {loadingRepos ? "…" : "Search"}
        </Button>
      </div>

      <div>
        <label htmlFor="forge-repo-pick" className="sr-only">
          Repository
        </label>
        <Select
          id="forge-repo-pick"
          value={repo}
          onChange={(e) => setRepo(e.target.value)}
        >
          <option value="">Select a repository…</option>
          {repos.map((r) => (
            <option key={r.full_name} value={r.full_name} disabled={!r.can_admin}>
              {r.full_name}
              {r.can_admin ? "" : " (no admin access)"}
            </option>
          ))}
        </Select>
      </div>

      <div>
        <div className="text-xs uppercase tracking-wider text-fg-muted mb-1">Bots to enable</div>
        {repoBots.length === 0 ? (
          <div className="text-fg-muted text-sm">
            No repo-installable bots found (a bot needs an{" "}
            <span className="font-mono">invocations:</span> block in its manifest).
          </div>
        ) : (
          <div className="space-y-3">
            {GROUP_ORDER.map((group) => {
              const inGroup = repoBots.filter((b) => primaryGroup(b) === group);
              if (inGroup.length === 0) return null;
              return (
                <div key={group}>
                  <div className="text-caption text-fg-muted mb-1">{GROUP_LABELS[group]}</div>
                  <ul className="space-y-2">
                    {inGroup.map((b) => (
                      <li key={b.name} className="flex gap-2">
                        <Checkbox
                          id={`fb-${b.name}`}
                          checked={selectedBots.includes(b.name)}
                          onChange={() => toggleBot(b.name)}
                          className="mt-1"
                        />
                        <label htmlFor={`fb-${b.name}`} className="text-sm">
                          <span className="font-medium">{b.display_name || b.name}</span>{" "}
                          <span className="font-mono text-fg-muted">{b.name}</span>
                          <span className="block mt-0.5 flex flex-wrap gap-1">
                            {triggerChips(b).map((c) => (
                              <span
                                key={c}
                                className="inline-block font-mono text-caption text-fg-muted bg-surface-1 border border-border-subtle rounded px-1"
                              >
                                {c}
                              </span>
                            ))}
                          </span>
                        </label>
                      </li>
                    ))}
                  </ul>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {preview && (
        <div className="bg-surface-1 border border-border-subtle rounded p-2 text-xs space-y-1">
          {preview.forge_native_events.length > 0 && (
            <div>
              <span className="text-fg-muted">Will subscribe to:</span>{" "}
              <span className="font-mono">{preview.forge_native_events.join(", ")}</span>
            </div>
          )}
          {preview.identity.handle && (
            <div>
              <span className="text-fg-muted">Will post as:</span> @{preview.identity.handle}
            </div>
          )}
          {hasConflicts &&
            preview.conflicts.map((c) => (
              <div key={c} className="text-danger">
                ⚠ {c}
              </div>
            ))}
        </div>
      )}

      <div className="flex items-center gap-2">
        <Button
          variant="primary"
          onClick={() => void enable()}
          disabled={busy || !repo || selectedBots.length === 0 || hasConflicts}
          loading={busy}
        >
          {busy ? "Enabling…" : "Enable"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
