import type { RunSourceKind, RunSummary } from "@/api/runs";

import {
  normalizeSourceKind,
  SOURCE_KIND_ORDER,
} from "./runSourceMeta";

// Date-range chip values. "all" keeps the previous behaviour where the
// list is unfiltered by creation time.
export type SinceFilter = "all" | "today" | "7d" | "30d";

// Source-filter chip values. "" is the "All" pseudo-value (no source
// filter applied), matching the existing status chip's contract.
export type SourceFilter = "" | RunSourceKind;

export const SINCE_FILTERS: Array<{ value: SinceFilter; label: string }> = [
  { value: "all", label: "All time" },
  { value: "today", label: "Today" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

// parseSince normalises the URL query value into a SinceFilter. Any
// unknown value falls back to "all" so a stale bookmark never wedges
// the page.
export function parseSince(raw: string | null | undefined): SinceFilter {
  switch (raw) {
    case "today":
    case "7d":
    case "30d":
      return raw;
    default:
      return "all";
  }
}

// parseSource normalises the `?source=` URL value into a SourceFilter.
// Empty / unknown values fall back to "" ("All") so a stale bookmark
// can't pin the view to a kind that no longer exists.
export function parseSource(raw: string | null | undefined): SourceFilter {
  switch (raw) {
    case "manual":
    case "webhook":
    case "dispatcher":
    case "fork":
    case "shard":
      return raw;
    default:
      return "";
  }
}

// sinceCutoff returns the earliest acceptable created_at as an epoch
// ms, or null when no time filter is active. "today" anchors on the
// caller-supplied `now` so tests are deterministic.
export function sinceCutoff(filter: SinceFilter, now: number): number | null {
  if (filter === "all") return null;
  if (filter === "today") {
    const d = new Date(now);
    d.setHours(0, 0, 0, 0);
    return d.getTime();
  }
  const days = filter === "7d" ? 7 : 30;
  return now - days * 24 * 60 * 60 * 1000;
}

// matchesQuery does a case-insensitive substring match across the
// fields a user would think to type: workflow display name, file
// path, run id, and any custom run name.
function matchesQuery(run: RunSummary, needle: string): boolean {
  const haystack = [
    run.name,
    run.workflow_name,
    run.bundle_name,
    run.file_path,
    run.id,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(needle);
}

export interface FilterOptions {
  query: string;
  since: SinceFilter;
  // Source classifier filter ("" = all kinds). Defaults to "" so
  // existing call sites stay source-agnostic.
  source?: SourceFilter;
  // Injected for deterministic tests; defaults to Date.now() in
  // production callers.
  now?: number;
}

// filterRuns applies the search box + date chip + source filters in one
// pass. Status filtering is intentionally out of scope here — the
// server already does it via `useRuns({ status })`, and we want this
// helper to operate on whatever the hook returns.
export function filterRuns(
  runs: RunSummary[],
  { query, since, source = "", now = Date.now() }: FilterOptions,
): RunSummary[] {
  const needle = query.trim().toLowerCase();
  const cutoff = sinceCutoff(since, now);
  if (!needle && cutoff === null && source === "") return runs;
  return runs.filter((r) => {
    if (needle && !matchesQuery(r, needle)) return false;
    if (cutoff !== null) {
      const t = Date.parse(r.created_at);
      if (Number.isNaN(t) || t < cutoff) return false;
    }
    if (source !== "" && normalizeSourceKind(r.source_kind) !== source) return false;
    return true;
  });
}

// availableSourceKinds returns the (ordered) subset of source kinds
// present in the fetched list. The Source chip row uses it to hide
// kinds that would never produce a hit — keeps the chip strip tight
// on small projects.
export function availableSourceKinds(runs: RunSummary[]): RunSourceKind[] {
  const seen = new Set<RunSourceKind>();
  for (const r of runs) seen.add(normalizeSourceKind(r.source_kind));
  return SOURCE_KIND_ORDER.filter((k) => seen.has(k));
}
