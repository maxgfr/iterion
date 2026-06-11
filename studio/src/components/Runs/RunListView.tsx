import { memo, useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useLocation, useSearch } from "wouter";

import {
  BarChartIcon,
  Cross2Icon,
  MagnifyingGlassIcon,
  ReloadIcon,
  RocketIcon,
} from "@radix-ui/react-icons";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { IconButton } from "@/components/ui/IconButton";
import { Input } from "@/components/ui/Input";
import { LiveDot } from "@/components/ui/LiveDot";
import { Select } from "@/components/ui/Select";
import type { RunSourceKind, RunStatus, RunSummary } from "@/api/runs";
import { formatRelative } from "@/lib/format";
import { useDebounce } from "@/hooks/useDebounce";
import { useRuns } from "@/hooks/useRuns";
import { STATUS_VARIANT, labelForStatus } from "./runStatusMeta";
import { metaForSource, runSourceKind } from "./runSourceMeta";
import QueueDepthBar from "./QueueDepthBar";
import {
  availableSourceKinds,
  filterRuns,
  parseSince,
  parseSource,
  SINCE_FILTERS,
  type SinceFilter,
  type SourceFilter,
} from "./runListFilter";
import {
  GROUP_OPTIONS,
  groupRuns,
  parseGroup,
  parseSort,
  SORT_OPTIONS,
  sortRuns,
  type GroupKey,
  type SortKey,
} from "./runListSortGroup";

const STATUS_FILTERS: Array<{ value: RunStatus | ""; label: string }> = [
  { value: "", label: "All" },
  { value: "running", label: "Running" },
  // queued sits between running and paused so the eye walks the
  // progression naturally (cloud-ready plan §F T-13).
  { value: "queued", label: "Queued" },
  { value: "paused_waiting_human", label: "Paused" },
  // Operator soft-pause has its own status; the row above is the
  // human-input variant. Keep both addressable in the filter strip so
  // an operator triaging "what's paused" can disambiguate at-a-glance.
  { value: "paused_operator", label: "Paused (operator)" },
  { value: "finished", label: "Finished" },
  { value: "failed", label: "Failed" },
  { value: "failed_resumable", label: "Failed (resumable)" },
  { value: "cancelled", label: "Cancelled" },
];

const QUERY_DEBOUNCE_MS = 150;

export default function RunListView() {
  const [location, setLocation] = useLocation();
  const search = useSearch();
  // Initial state derives from the URL so reload + browser-back
  // restore the user's filters. After mount we drive the URL from
  // state, never the reverse.
  const initial = useMemo(() => {
    const p = new URLSearchParams(search);
    return {
      query: p.get("q") ?? "",
      since: parseSince(p.get("since")),
      source: parseSource(p.get("source")),
      sort: parseSort(p.get("sort")),
      group: parseGroup(p.get("group")),
    };
    // Mount-only: subsequent search-string changes come from our own
    // setLocation calls below and must not clobber in-flight state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const [status, setStatus] = useState<RunStatus | "">("");
  const [queryInput, setQueryInput] = useState(initial.query);
  const query = useDebounce(queryInput, QUERY_DEBOUNCE_MS);
  const [since, setSince] = useState<SinceFilter>(initial.since);
  const [source, setSource] = useState<SourceFilter>(initial.source);
  const [sort, setSort] = useState<SortKey>(initial.sort);
  const [group, setGroup] = useState<GroupKey>(initial.group);

  // Mirror committed (debounced) filters into the URL with replace
  // semantics — keeps the back-button stack uncluttered while still
  // making the page bookmarkable. Status is intentionally not part
  // of the URL contract yet (kept consistent with prior behaviour).
  useEffect(() => {
    const p = new URLSearchParams();
    if (query) p.set("q", query);
    if (since !== "all") p.set("since", since);
    if (source !== "") p.set("source", source);
    if (sort !== "started") p.set("sort", sort);
    if (group !== "none") p.set("group", group);
    const qs = p.toString();
    const target = qs ? `/runs?${qs}` : "/runs";
    const current = search ? `${location}?${search}` : location;
    if (target !== current) setLocation(target, { replace: true });
  }, [query, since, source, sort, group, setLocation, location, search]);

  const { runs, counts, loading, error } = useRuns({ status });

  const filteredRuns = useMemo(
    () => filterRuns(runs, { query, since, source }),
    [runs, query, since, source],
  );

  // Source-filter chip strip: only show kinds present in the current
  // (status-filtered) fetched list. Recomputed off `runs` (not the
  // post-filter list) so picking "Webhook" doesn't make the other
  // chips vanish.
  const availableSources = useMemo(() => availableSourceKinds(runs), [runs]);

  // Per-source counts for the chip strip — informational, mirrors the
  // status chip's count rendering.
  const sourceCounts = useMemo(() => {
    const m: Partial<Record<RunSourceKind, number>> = {};
    for (const r of runs) {
      const k = runSourceKind(r);
      m[k] = (m[k] ?? 0) + 1;
    }
    return m;
  }, [runs]);

  const openRun = useCallback(
    (id: string) => setLocation(`/runs/${encodeURIComponent(id)}`),
    [setLocation],
  );

  // Force a re-render once per second while at least one visible run
  // is still in-flight (no finished_at), so the duration column ticks
  // forward instead of freezing on whatever value the last poll
  // produced. Idle when every visible run has finished.
  const hasLiveRun = useMemo(
    () => filteredRuns.some((r) => !r.finished_at),
    [filteredRuns],
  );
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!hasLiveRun) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [hasLiveRun]);
  // Read tick so React preserves the dependency edge — the explicit
  // void prevents the linter from treating it as dead.
  void tick;

  // Sort + group are layered on top of the filtered list. We anchor
  // the "duration" sort and the in-flight tick on a captured `now` so
  // re-renders within the same tick produce a stable order.
  const now = useMemo(() => Date.now(), [tick]);
  const sortedRuns = useMemo(
    () => sortRuns(filteredRuns, sort, now),
    [filteredRuns, sort, now],
  );
  const groups = useMemo(() => groupRuns(sortedRuns, group), [sortedRuns, group]);
  const isGrouped = group !== "none";

  const filtersActive =
    query !== "" || since !== "all" || source !== "";
  const clearFilters = useCallback(() => {
    setQueryInput("");
    setSince("all");
    setSource("");
  }, []);

  return (
    <div className="h-full flex flex-col overflow-hidden bg-surface-1 text-fg-default">
      <QueueDepthBar counts={counts} />

      <div className="px-4 py-2 flex flex-col gap-2 border-b border-border-default">
        <div className="flex items-center gap-2">
          <div className="flex-1 max-w-md">
            <Input
              type="search"
              value={queryInput}
              onChange={(e) => setQueryInput(e.currentTarget.value)}
              placeholder="Search name, workflow, file path, run id…"
              leadingIcon={<MagnifyingGlassIcon />}
              aria-label="Search runs"
            />
          </div>
          <div className="flex items-center gap-1.5">
            {SINCE_FILTERS.map((f) => (
              <FilterChip
                key={f.value}
                active={since === f.value}
                label={f.label}
                onClick={() => setSince(f.value)}
              />
            ))}
          </div>
          {filtersActive && (
            <IconButton
              label="Clear filters"
              size="sm"
              variant="ghost"
              onClick={clearFilters}
            >
              <Cross2Icon />
            </IconButton>
          )}
          <div className="ml-auto flex items-center gap-2">
            <SortGroupControls
              sort={sort}
              onSort={setSort}
              group={group}
              onGroup={setGroup}
            />
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<BarChartIcon />}
              onClick={() => setLocation("/insights")}
              title="Cross-run cost, fail rate, and duration over a configurable window"
            >
              Analytics
            </Button>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {STATUS_FILTERS.map((f) => {
            const count =
              f.value === ""
                ? runs.length
                : counts[f.value as RunStatus] ?? 0;
            return (
              <FilterChip
                key={f.value || "all"}
                active={status === f.value}
                label={f.label}
                count={count > 0 ? count : undefined}
                size="sm"
                onClick={() => setStatus(f.value)}
              />
            );
          })}
        </div>
        {availableSources.length > 0 && (
          <div
            className="flex flex-wrap items-center gap-1.5"
            aria-label="Filter by run source"
          >
            <span className="text-fg-subtle text-xs mr-0.5">Source</span>
            <FilterChip
              active={source === ""}
              label="All"
              count={runs.length > 0 ? runs.length : undefined}
              size="sm"
              onClick={() => setSource("")}
            />
            {availableSources.map((kind) => {
              const meta = metaForSource(kind);
              const Icon = meta.Icon;
              return (
                <FilterChip
                  key={kind}
                  active={source === kind}
                  label={
                    <span
                      className="inline-flex items-center gap-1"
                      title={meta.description}
                    >
                      <Icon className="w-3 h-3" />
                      <span>{meta.label}</span>
                    </span>
                  }
                  count={sourceCounts[kind]}
                  size="sm"
                  onClick={() => setSource(kind)}
                />
              );
            })}
          </div>
        )}
      </div>

      <div className="flex-1 overflow-auto">
        {loading && runs.length === 0 ? (
          <EmptyState
            message="Loading runs…"
            icon={<ReloadIcon className="animate-spin" />}
          />
        ) : error ? (
          <EmptyState message={<span className="text-danger">{error}</span>} />
        ) : runs.length === 0 ? (
          <EmptyState
            title="No runs yet"
            message="Launch a workflow from the editor to populate this list."
            action={
              <Button
                variant="primary"
                size="sm"
                leadingIcon={<RocketIcon />}
                onClick={() => setLocation("/editor")}
              >
                Open editor
              </Button>
            }
            secondaryAction={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setLocation("/")}
              >
                Home
              </Button>
            }
          />
        ) : filteredRuns.length === 0 ? (
          status !== "" ? (
            <EmptyState
              title={`No ${statusFilterLabel(status)} runs`}
              message="Pick a different status, or clear the filters to see everything."
              action={
                <Button variant="secondary" size="sm" onClick={() => setStatus("")}>
                  Show all statuses
                </Button>
              }
              secondaryAction={
                filtersActive ? (
                  <Button variant="ghost" size="sm" onClick={clearFilters}>
                    Clear search / date
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <EmptyState
              title="No matching runs"
              message="Try a different search term or widen the date range."
              action={
                <Button variant="secondary" size="sm" onClick={clearFilters}>
                  Clear filters
                </Button>
              }
            />
          )
        ) : (
          <>
            {/* Desktop / tablet: standard table. Adds a Source column
                so the user can tell at a glance how each run was
                triggered without expanding the row. */}
            <table className="w-full text-xs hidden sm:table">
              <thead className="text-fg-subtle">
                <tr className="border-b border-border-default">
                  <th className="text-left px-4 py-2 font-medium">Run</th>
                  <th className="text-left px-4 py-2 font-medium">Workflow</th>
                  <th className="text-left px-4 py-2 font-medium">Source</th>
                  <th className="text-left px-4 py-2 font-medium">Status</th>
                  <th className="text-left px-4 py-2 font-medium">Started</th>
                  <th className="text-left px-4 py-2 font-medium">Duration</th>
                  <th className="text-left px-4 py-2 font-medium">Run ID</th>
                </tr>
              </thead>
              <tbody>
                {groups.map((g) => (
                  <RunRowGroup
                    key={g.id}
                    label={g.label}
                    count={g.runs.length}
                    showHeader={isGrouped}
                    columnSpan={7}
                  >
                    {g.runs.map((r) => (
                      <RunRow key={r.id} run={r} onOpen={openRun} />
                    ))}
                  </RunRowGroup>
                ))}
              </tbody>
            </table>
            <div className="sm:hidden">
              {groups.map((g) => (
                <div key={g.id}>
                  {isGrouped && (
                    <RunCardGroupHeader label={g.label} count={g.runs.length} />
                  )}
                  <ul className="divide-y divide-border-default">
                    {g.runs.map((r) => (
                      <li key={r.id}>
                        <RunCard run={r} onOpen={openRun} />
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function FilterChip({
  active,
  label,
  count,
  size = "md",
  onClick,
}: {
  active: boolean;
  label: ReactNode;
  count?: number;
  size?: "sm" | "md";
  onClick: () => void;
}) {
  const heightCls = size === "sm" ? "h-6" : "h-7";
  const activeCls = active
    ? "border-accent/40 bg-accent-soft text-fg-default"
    : "border-border-default bg-surface-2 text-fg-default hover:bg-surface-3";
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1 rounded-md border text-xs px-2 ${heightCls} ${activeCls}`}
    >
      {label}
      {count !== undefined && <span className="text-fg-subtle">{count}</span>}
    </button>
  );
}

// Memoised so the parent's per-row callback (now stable via useCallback)
// doesn't force every row to re-render when one run mutates.
const RunRow = memo(function RunRow({
  run,
  onOpen,
}: {
  run: RunSummary;
  onOpen: (id: string) => void;
}) {
  return (
    <tr
      className="border-b border-border-default hover:bg-surface-2 cursor-pointer"
      onClick={() => onOpen(run.id)}
    >
      <td className="px-4 py-2">
        <div className="font-medium">{friendlyLabel(run)}</div>
      </td>
      <td className="px-4 py-2">
        {workflowDisplay(run) && (
          <div className="text-fg-default">{workflowDisplay(run)}</div>
        )}
        {run.file_path && (
          <div className="text-fg-subtle text-[10px] truncate max-w-md">
            {run.file_path}
          </div>
        )}
      </td>
      <td className="px-4 py-2">
        <SourceBadge run={run} />
      </td>
      <td className="px-4 py-2">
        <Badge variant={STATUS_VARIANT[run.status]}>
          {labelForStatus(run.status)}
        </Badge>
        {run.active && (
          <LiveDot
            tone="live"
            size="sm"
            className="ml-1.5"
            label="Active in this process"
          />
        )}
      </td>
      <td className="px-4 py-2 text-fg-muted">{formatRelative(run.created_at)}</td>
      <td className="px-4 py-2 text-fg-muted">
        {formatDuration(run.created_at, run.finished_at)}
      </td>
      <td className="px-4 py-2 font-mono text-[10px] text-fg-subtle" title={run.id}>
        {shortRunID(run.id)}
      </td>
    </tr>
  );
});

const RunCard = memo(function RunCard({
  run,
  onOpen,
}: {
  run: RunSummary;
  onOpen: (id: string) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onOpen(run.id)}
      className="w-full text-left px-4 py-3 flex flex-col gap-1 min-h-[44px] hover:bg-surface-2 active:bg-surface-3"
    >
      <div className="flex items-center gap-2 min-w-0 flex-wrap">
        <Badge variant={STATUS_VARIANT[run.status]}>
          {labelForStatus(run.status)}
        </Badge>
        <SourceBadge run={run} />
        {run.active && (
          <LiveDot tone="live" size="sm" label="Active in this process" />
        )}
        <span className="font-medium truncate">
          {friendlyLabel(run)}
        </span>
      </div>
      {workflowDisplay(run) && (
        <div className="text-[11px] text-fg-default truncate">
          {workflowDisplay(run)}
        </div>
      )}
      <div className="text-[11px] text-fg-muted flex flex-wrap gap-x-2">
        <span>{formatRelative(run.created_at)}</span>
        <span>·</span>
        <span>{formatDuration(run.created_at, run.finished_at)}</span>
      </div>
      <div
        className="text-[10px] text-fg-subtle font-mono truncate"
        title={run.id}
      >
        {shortRunID(run.id)}
      </div>
    </button>
  );
});

// SourceBadge renders the derived source classification for a run.
// Empty / unknown source_kind values normalise to "manual" so legacy
// rows still get a glyph instead of an awkward blank cell.
const SourceBadge = memo(function SourceBadge({ run }: { run: RunSummary }) {
  const kind = runSourceKind(run);
  const meta = metaForSource(kind);
  const Icon = meta.Icon;
  return (
    <Badge
      variant={meta.variant}
      size="sm"
      title={meta.description}
      leadingIcon={<Icon className="w-3 h-3" />}
    >
      {meta.label}
    </Badge>
  );
});

// SortGroupControls renders the two compact <select> dropdowns that
// drive the client-side sort + grouping axes. Stateless: the parent
// owns the URL-synced state. Hidden on the tightest viewports — at
// that width the search box + chip strips already saturate the toolbar
// row, and the dropdowns wrap awkwardly.
function SortGroupControls({
  sort,
  onSort,
  group,
  onGroup,
}: {
  sort: SortKey;
  onSort: (next: SortKey) => void;
  group: GroupKey;
  onGroup: (next: GroupKey) => void;
}) {
  return (
    <div className="hidden md:flex items-center gap-1.5">
      <label className="text-fg-subtle text-xs flex items-center gap-1">
        <span>Sort</span>
        <Select
          size="sm"
          value={sort}
          onChange={(e) => onSort(e.currentTarget.value as SortKey)}
          aria-label="Sort runs"
          className="w-32"
        >
          {SORT_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </Select>
      </label>
      <label className="text-fg-subtle text-xs flex items-center gap-1">
        <span>Group</span>
        <Select
          size="sm"
          value={group}
          onChange={(e) => onGroup(e.currentTarget.value as GroupKey)}
          aria-label="Group runs"
          className="w-32"
        >
          {GROUP_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </Select>
      </label>
    </div>
  );
}

// RunRowGroup renders one group's rows inside the desktop <table>. When
// the active group key is "none", the header row is suppressed and
// only the rows pass through — keeps the table identical to the
// pre-feature layout.
function RunRowGroup({
  label,
  count,
  showHeader,
  columnSpan,
  children,
}: {
  label: string;
  count: number;
  showHeader: boolean;
  columnSpan: number;
  children: ReactNode;
}) {
  if (!showHeader) {
    // Fragments in <tbody> render directly as a child sequence — no
    // wrapping element, so the rows keep their normal striping.
    return <>{children}</>;
  }
  return (
    <>
      <tr className="bg-surface-2 border-y border-border-default">
        <th
          colSpan={columnSpan}
          scope="rowgroup"
          className="text-left px-4 py-1.5 font-medium text-fg-muted text-micro uppercase tracking-wide"
        >
          <span>{label}</span>
          <span className="ml-2 text-fg-subtle normal-case tracking-normal">
            {count}
          </span>
        </th>
      </tr>
      {children}
    </>
  );
}

// RunCardGroupHeader is the mobile-list counterpart to RunRowGroup —
// rendered above each group's <ul>. Visually subdued so the rows still
// dominate the scroll.
function RunCardGroupHeader({ label, count }: { label: string; count: number }) {
  return (
    <div className="px-4 py-1.5 bg-surface-2 border-y border-border-default text-fg-muted text-micro uppercase tracking-wide">
      <span>{label}</span>
      <span className="ml-2 text-fg-subtle normal-case tracking-normal">
        {count}
      </span>
    </div>
  );
}

// hasFriendlyName returns true when run.name is set AND differs from
// run.id. Defensive guard against historical bugs where dispatcher-
// spawned runs aliased Name to the composite RunID (now fixed — see
// pkg/dispatcher/loop.go); legacy stores may still contain such rows.
function hasFriendlyName(run: RunSummary): boolean {
  return Boolean(run.name) && run.name !== run.id;
}

// friendlyLabel returns the per-run instance label for the "Run"
// column. Falls back to workflow_name for legacy runs (persisted
// before the friendly-name feature shipped).
function friendlyLabel(run: RunSummary): string {
  return hasFriendlyName(run) ? run.name! : run.workflow_name;
}

// workflowDisplay returns the label for the "Workflow" column. Returns
// "" when the value would duplicate the run id (dispatcher-spawned
// runs in some legacy paths aliased workflow_name to the composite
// run id; suppress to keep the row from showing the id twice).
function workflowDisplay(run: RunSummary): string {
  const name = run.bundle_name || run.workflow_name;
  if (!name || name === run.id) return "";
  return name;
}

// shortRunID collapses a long run id to a glanceable prefix. Keeps
// the tooltip-attached full id within reach via `title=`. For the
// dispatcher's composite ids (e.g. `dispatcher-native_<uuid>-<seq>-<ts>`),
// we surface the prefix + the first UUID segment so two siblings
// from the same dispatcher slot are still visually distinct.
function shortRunID(id: string): string {
  if (!id) return "";
  const dispPrefix = "dispatcher-native_";
  if (id.startsWith(dispPrefix)) {
    const tail = id.slice(dispPrefix.length);
    const dash = tail.indexOf("-");
    return `disp:${dash > 0 ? tail.slice(0, dash) : tail.slice(0, 8)}`;
  }
  return id.length > 14 ? id.slice(0, 14) + "…" : id;
}

// statusFilterLabel returns a lower-case fragment suited for the
// "No <status> runs" empty headline (matches the chip label so the
// phrasing stays consistent between the chip and the empty state).
// Exported for the corresponding unit test; not consumed elsewhere.
export function statusFilterLabel(status: RunStatus | ""): string {
  if (status === "") return "matching";
  const entry = STATUS_FILTERS.find((f) => f.value === status);
  const label = entry?.label ?? labelForStatus(status);
  return label.toLowerCase();
}

function formatDuration(startISO: string, endISO?: string): string {
  const start = Date.parse(startISO);
  if (Number.isNaN(start)) return "";
  const end = endISO ? Date.parse(endISO) : Date.now();
  if (Number.isNaN(end)) return "";
  const ms = Math.max(0, end - start);
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remSec = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remSec}s`;
  const hours = Math.floor(minutes / 60);
  const remMin = minutes % 60;
  return `${hours}h ${remMin}m`;
}
