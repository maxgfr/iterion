import { memo, useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useLocation, useSearch } from "wouter";

import {
  BarChartIcon,
  CheckIcon,
  ChevronDownIcon,
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
import { Popover } from "@/components/ui/Popover";
import { Select } from "@/components/ui/Select";
import type { RunRepo, RunSourceKind, RunStatus, RunSummary } from "@/api/runs";
import { formatRelative } from "@/lib/format";
import { useDebounce } from "@/hooks/useDebounce";
import { useRuns } from "@/hooks/useRuns";
import { useRunRepos } from "@/hooks/useRunRepos";
import { useServerInfoStore } from "@/store/serverInfo";
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
import { availableBots, botEmoji, botLabel, type BotDescriptor } from "./runBotMeta";
import {
  availableRepos,
  repoAxisLabel,
  type RepoChip,
  type RunMode,
} from "./runRepoMeta";
import {
  groupOptionsFor,
  groupRuns,
  parseGroup,
  parseSort,
  SORT_OPTIONS,
  sortRuns,
  workflowLabel,
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
      bot: p.get("bot") ?? "",
      repo: p.get("repo") ?? "",
      sort: parseSort(p.get("sort")),
      group: parseGroup(p.get("group")),
    };
    // Mount-only: subsequent search-string changes come from our own
    // setLocation calls below and must not clobber in-flight state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Server mode selects the repo axis: cloud filters by repository
  // (project_path, server-side); local/desktop filters by folder
  // (repo_root||work_dir, client-side). Anything non-cloud is "local".
  const serverMode = useServerInfoStore((s) => s.info?.mode);
  const mode: RunMode = serverMode === "cloud" ? "cloud" : "local";

  const [status, setStatus] = useState<RunStatus | "">("");
  const [queryInput, setQueryInput] = useState(initial.query);
  const query = useDebounce(queryInput, QUERY_DEBOUNCE_MS);
  const [since, setSince] = useState<SinceFilter>(initial.since);
  const [source, setSource] = useState<SourceFilter>(initial.source);
  const [bot, setBot] = useState<string>(initial.bot);
  const [repo, setRepo] = useState<string>(initial.repo);
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
    if (bot !== "") p.set("bot", bot);
    if (repo !== "") p.set("repo", repo);
    if (sort !== "started") p.set("sort", sort);
    if (group !== "none") p.set("group", group);
    const qs = p.toString();
    const target = qs ? `/runs?${qs}` : "/runs";
    const current = search ? `${location}?${search}` : location;
    if (target !== current) setLocation(target, { replace: true });
  }, [query, since, source, bot, repo, sort, group, setLocation, location, search]);

  // The repo axis splits by mode: cloud filters server-side (index-backed,
  // a project_path slug), local filters client-side (a folder path that is
  // not a project_path and would match nothing server-side). Resolve the
  // split once so neither the fetch nor filterRuns has to know about mode.
  const serverRepo = mode === "cloud" ? repo : "";
  const clientRepo = mode === "cloud" ? "" : repo;

  const { runs, counts, loading, error } = useRuns({ status, repo: serverRepo });

  const filteredRuns = useMemo(
    () => filterRuns(runs, { query, since, source, bot, repo: clientRepo }),
    [runs, query, since, source, bot, clientRepo],
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

  // Bot-filter chip strip: distinct bots (with counts) present in the
  // fetched list — computed off `runs` (not the post-filter list) so
  // picking one bot doesn't hide the others. An active bot with no
  // surviving runs is prepended so it stays visible and clearable.
  const botChips = useMemo<BotDescriptor[]>(() => {
    const base = availableBots(runs);
    if (bot === "" || base.some((b) => b.key === bot)) return base;
    return [{ key: bot, label: bot, emoji: "🤖", count: 0 }, ...base];
  }, [runs, bot]);

  // Repo/folder chip strip. Cloud: the index-backed distinct-repos
  // endpoint, decoupled from the list so selecting a repo (which narrows
  // the server-side fetch) doesn't empty the strip. Local: derived
  // client-side from the fetched runs' folders. An active value absent
  // from the set is prepended so it stays clearable.
  const { repos: cloudRepos } = useRunRepos(mode === "cloud");
  const repoChips = useMemo<RepoChip[]>(() => {
    const base: RepoChip[] =
      mode === "cloud"
        ? cloudRepos.map((r: RunRepo) => ({
            key: r.project_path,
            label: r.project_path,
            title: r.project_path,
            count: r.count,
          }))
        : availableRepos(runs);
    if (repo === "" || base.some((c) => c.key === repo)) return base;
    return [{ key: repo, label: repo, title: repo, count: 0 }, ...base];
  }, [mode, cloudRepos, runs, repo]);

  const openRun = useCallback(
    (id: string) => setLocation(`/runs/${encodeURIComponent(id)}`),
    [setLocation],
  );

  // Click a row's bot avatar to filter to that bot — or toggle it off
  // when it's already the active bot. Stable so memoised rows don't
  // re-render when other state changes.
  const filterByBot = useCallback(
    (key: string) => setBot((prev) => (prev === key ? "" : key)),
    [],
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

  // Shared "All" option count for the source + bot menus (the full
  // status-filtered fetch size). Omitted when zero.
  const totalCount = runs.length > 0 ? runs.length : undefined;

  // Per-axis menu option lists. Status/Since are tiny and built inline in
  // the JSX; the data-driven axes are memoised because they map over
  // potentially many bots/repos and build label nodes.
  const sourceOptions = useMemo<FilterMenuOption[]>(
    () => [
      { key: "", label: "All", count: totalCount },
      ...availableSources.map((kind) => {
        const meta = metaForSource(kind);
        const Icon = meta.Icon;
        return {
          key: kind,
          label: (
            <span className="inline-flex items-center gap-1.5" title={meta.description}>
              <Icon className="w-3 h-3" />
              <span>{meta.label}</span>
            </span>
          ),
          count: sourceCounts[kind],
        };
      }),
    ],
    [availableSources, sourceCounts, totalCount],
  );

  const botOptions = useMemo<FilterMenuOption[]>(
    () => [
      { key: "", label: "All", count: totalCount },
      ...botChips.map((b) => ({
        key: b.key,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <span>{b.emoji}</span>
            <span>{b.label}</span>
          </span>
        ),
        count: b.count > 0 ? b.count : undefined,
      })),
    ],
    [botChips, totalCount],
  );

  const repoOptions = useMemo<FilterMenuOption[]>(
    () => [
      { key: "", label: "All" },
      ...repoChips.map((c) => ({
        key: c.key,
        label: c.label,
        count: c.count > 0 ? c.count : undefined,
      })),
    ],
    [repoChips],
  );

  const filtersActive =
    query !== "" ||
    since !== "all" ||
    source !== "" ||
    bot !== "" ||
    repo !== "";
  const clearFilters = useCallback(() => {
    setQueryInput("");
    setSince("all");
    setSource("");
    setBot("");
    setRepo("");
  }, []);

  return (
    <div className="h-full flex flex-col overflow-hidden bg-surface-1 text-fg-default">
      <QueueDepthBar counts={counts} />

      <div className="px-4 py-2 flex flex-wrap items-center gap-2 border-b border-border-default">
        <div className="flex-1 min-w-48 max-w-md">
          <Input
            type="search"
            value={queryInput}
            onChange={(e) => setQueryInput(e.currentTarget.value)}
            placeholder="Search name, workflow, file path, run id…"
            leadingIcon={<MagnifyingGlassIcon />}
            aria-label="Search runs"
          />
        </div>

        <FilterMenu
          axis="Status"
          ariaLabel="Filter by status"
          value={status}
          options={STATUS_FILTERS.map((f) => {
            const n =
              f.value === "" ? runs.length : counts[f.value as RunStatus] ?? 0;
            return { key: f.value, label: f.label, count: n > 0 ? n : undefined };
          })}
          onSelect={(k) => setStatus(k as RunStatus | "")}
        />
        {availableSources.length > 0 && (
          <FilterMenu
            axis="Source"
            ariaLabel="Filter by run source"
            value={source}
            options={sourceOptions}
            onSelect={(k) => setSource(k as SourceFilter)}
          />
        )}
        {(botChips.length > 1 || bot !== "") && (
          <FilterMenu
            axis="Bot"
            ariaLabel="Filter by bot"
            value={bot}
            options={botOptions}
            onSelect={setBot}
          />
        )}
        {(repoChips.length > 1 || repo !== "") && (
          <FilterMenu
            axis={repoAxisLabel(mode)}
            ariaLabel={`Filter by ${repoAxisLabel(mode).toLowerCase()}`}
            value={repo}
            options={repoOptions}
            onSelect={setRepo}
          />
        )}
        <FilterMenu
          axis="Since"
          ariaLabel="Filter by date"
          value={since}
          defaultValue="all"
          options={SINCE_FILTERS.map((f) => ({ key: f.value, label: f.label }))}
          onSelect={(k) => setSince(k as SinceFilter)}
        />
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
            groupOptions={groupOptionsFor(mode)}
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
                      <RunRow
                        key={r.id}
                        run={r}
                        onOpen={openRun}
                        onFilterBot={filterByBot}
                      />
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
                        <RunCard run={r} onOpen={openRun} onFilterBot={filterByBot} />
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

// One option in a FilterMenu. `key` is the filter value ("" = the
// unset/"All" entry); `label` is a (possibly icon/emoji-laden) node;
// `count` is an optional right-aligned tally.
interface FilterMenuOption {
  key: string;
  label: ReactNode;
  count?: number;
}

// FilterMenu is the per-axis dropdown pill used by the Status, Source,
// Bot, Repo/Folder, and Since filters. The trigger shows the axis name
// and, when a non-default value is selected, the active option's label
// (and an accent highlight). The popover lists the options with a check
// on the active one. Selecting closes the menu.
function FilterMenu({
  axis,
  ariaLabel,
  value,
  defaultValue = "",
  options,
  onSelect,
}: {
  axis: string;
  ariaLabel: string;
  value: string;
  defaultValue?: string;
  options: FilterMenuOption[];
  onSelect: (key: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const active = value !== defaultValue;
  const activeOption = active ? options.find((o) => o.key === value) : undefined;
  const triggerCls = active
    ? "border-accent/40 bg-accent-soft text-fg-default"
    : "border-border-default bg-surface-2 text-fg-muted hover:bg-surface-3";
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      contentClassName="p-1"
      trigger={
        <button
          type="button"
          aria-label={ariaLabel}
          className={`inline-flex items-center gap-1 rounded-md border text-xs h-7 px-2 ${triggerCls}`}
        >
          <span>{axis}</span>
          {activeOption && (
            <span className="flex items-center gap-1 max-w-40 truncate text-fg-default">
              <span className="text-fg-subtle">:</span>
              {activeOption.label}
            </span>
          )}
          <ChevronDownIcon className="w-3 h-3 opacity-60" />
        </button>
      }
    >
      <div role="listbox" aria-label={ariaLabel} className="min-w-44 max-h-72 overflow-auto">
        {options.map((o) => {
          const selected = o.key === value;
          return (
            <button
              key={o.key || "__all"}
              type="button"
              role="option"
              aria-selected={selected}
              onClick={() => {
                onSelect(o.key);
                setOpen(false);
              }}
              className={`w-full flex items-center gap-2 rounded px-2 h-7 text-xs text-left ${
                selected
                  ? "bg-accent-soft text-fg-default"
                  : "text-fg-default hover:bg-surface-2"
              }`}
            >
              <CheckIcon
                className={`w-3 h-3 shrink-0 text-accent ${selected ? "opacity-100" : "opacity-0"}`}
              />
              <span className="flex-1 truncate">{o.label}</span>
              {o.count !== undefined && (
                <span className="text-fg-subtle tabular-nums">{o.count}</span>
              )}
            </button>
          );
        })}
      </div>
    </Popover>
  );
}

// BotAvatar is the per-row bot glyph that doubles as a quick filter:
// clicking it filters the list to that bot (toggling off if already
// active). stopPropagation keeps the row's open-run click from firing.
const BotAvatar = memo(function BotAvatar({
  run,
  onFilter,
}: {
  run: RunSummary;
  onFilter: (botKey: string) => void;
}) {
  const key = workflowLabel(run);
  if (!key) return null;
  const label = botLabel(run);
  return (
    <button
      type="button"
      title={`Filter by ${label}`}
      aria-label={`Filter by ${label}`}
      onClick={(e) => {
        e.stopPropagation();
        onFilter(key);
      }}
      className="shrink-0 inline-flex items-center justify-center w-5 h-5 rounded text-sm leading-none hover:bg-surface-3"
    >
      <span aria-hidden>{botEmoji(run)}</span>
    </button>
  );
});

// Memoised so the parent's per-row callback (now stable via useCallback)
// doesn't force every row to re-render when one run mutates.
const RunRow = memo(function RunRow({
  run,
  onOpen,
  onFilterBot,
}: {
  run: RunSummary;
  onOpen: (id: string) => void;
  onFilterBot: (botKey: string) => void;
}) {
  return (
    <tr
      className="border-b border-border-default hover:bg-surface-2 cursor-pointer"
      onClick={() => onOpen(run.id)}
    >
      <td className="px-4 py-2">
        <div className="flex items-center gap-2">
          <BotAvatar run={run} onFilter={onFilterBot} />
          <span className="font-medium truncate">{friendlyLabel(run)}</span>
        </div>
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
  onFilterBot,
}: {
  run: RunSummary;
  onOpen: (id: string) => void;
  onFilterBot: (botKey: string) => void;
}) {
  // A div (not a <button>) so the bot-avatar button can nest validly;
  // keyboard semantics are restored via role + Enter/Space handling.
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onOpen(run.id)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen(run.id);
        }
      }}
      className="w-full text-left px-4 py-3 flex flex-col gap-1 min-h-[44px] cursor-pointer hover:bg-surface-2 active:bg-surface-3"
    >
      <div className="flex items-center gap-2 min-w-0 flex-wrap">
        <BotAvatar run={run} onFilter={onFilterBot} />
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
    </div>
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
  groupOptions,
}: {
  sort: SortKey;
  onSort: (next: SortKey) => void;
  group: GroupKey;
  onGroup: (next: GroupKey) => void;
  groupOptions: ReadonlyArray<{ value: GroupKey; label: string }>;
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
          {groupOptions.map((opt) => (
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
