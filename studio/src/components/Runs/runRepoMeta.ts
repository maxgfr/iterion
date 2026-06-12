import type { RunSummary } from "@/api/runs";
import { basename } from "@/lib/format";

// The repo/folder filter axis is one control whose *label* depends on
// the server mode: cloud filters by repository, local/desktop by folder.
// RunMode names that distinction. Note: only the UI affordances need it
// (repoAxisLabel, the group-dropdown label) — the per-run key/label below
// are mode-free, because a run carries exactly one identity: cloud runs
// have project_path, local runs have repo_root/work_dir.
export type RunMode = "cloud" | "local";

// One repo/folder present in the list — a chip on the filter strip.
// `key` is the stable filter value; `label` is what the chip shows;
// `title` is the hover tooltip (the full path/slug); `count` is how
// many runs match.
export interface RepoChip {
  key: string;
  label: string;
  title: string;
  count: number;
}

// repoAxisLabel is the strip header: "Repo" in cloud mode (repositories),
// "Folder" in local/desktop mode (filesystem directories).
export function repoAxisLabel(mode: RunMode): string {
  return mode === "cloud" ? "Repo" : "Folder";
}

// repoKey is the stable identifier a run is filtered/grouped by on the
// repo/folder axis: the cloud forge slug when present, else the git repo
// root, else the exec dir. A run only ever has one populated (project_path
// is empty in local mode; repo_root/work_dir are empty for cloud), so this
// needs no mode. Empty when the run carries none (a run launched outside
// any repo, or a legacy run) — such runs get no chip and are excluded by
// an active repo filter.
export function repoKey(run: RunSummary): string {
  return run.project_path || run.repo_root || run.work_dir || "";
}

// repoLabel is the chip text. A cloud slug (project_path) is already
// compact + meaningful, so it shows verbatim; a filesystem folder shows
// its basename (the full path is the tooltip via repoKey). Keyed on which
// field is populated, not on mode.
export function repoLabel(run: RunSummary): string {
  if (run.project_path) return run.project_path;
  const folder = run.repo_root || run.work_dir || "";
  return folder ? basename(folder) : "";
}

// availableRepos returns the distinct repos/folders present in the
// fetched list, with per-key counts, sorted by count desc then label
// asc (busiest first — mirrors the distinct-repos endpoint ordering).
// Used in LOCAL mode; cloud mode feeds its chips from the index-backed
// /api/v1/runs/repos endpoint instead (see useRunRepos).
export function availableRepos(runs: RunSummary[]): RepoChip[] {
  const byKey = new Map<string, RepoChip>();
  for (const run of runs) {
    const key = repoKey(run);
    if (!key) continue;
    const existing = byKey.get(key);
    if (existing) {
      existing.count++;
      continue;
    }
    byKey.set(key, { key, label: repoLabel(run), title: key, count: 1 });
  }
  return Array.from(byKey.values()).sort((a, b) => {
    if (a.count !== b.count) return b.count - a.count;
    return a.label.localeCompare(b.label, undefined, { sensitivity: "base" });
  });
}
