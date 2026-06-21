import { Cross2Icon, MagnifyingGlassIcon } from "@radix-ui/react-icons";

import type { RunStatus } from "@/api/runs";
import { IconButton } from "@/components/ui/IconButton";
import { Input } from "@/components/ui/Input";

import { repoAxisLabel, type RunMode } from "../runRepoMeta";
import {
  SINCE_FILTERS,
  type SinceFilter,
  type SourceFilter,
} from "../runListFilter";

import { FilterMenu, type FilterMenuOption } from "./FilterMenu";

// RunListFilters is the toolbar row that hosts the search input and
// the per-axis FilterMenus + the clear-filters affordance. Stateless:
// the parent owns the URL-synced state and the menu-option memoised
// data; this is a pure presentational slice.
export function RunListFilters({
  queryInput,
  onQueryChange,
  status,
  statusOptions,
  onStatus,
  source,
  sourceOptions,
  showSourceFilter,
  onSource,
  bot,
  botOptions,
  showBotFilter,
  onBot,
  repo,
  repoOptions,
  showRepoFilter,
  mode,
  onRepo,
  since,
  onSince,
  filtersActive,
  onClearFilters,
}: {
  queryInput: string;
  onQueryChange: (v: string) => void;
  status: RunStatus | "";
  statusOptions: FilterMenuOption[];
  onStatus: (k: RunStatus | "") => void;
  source: SourceFilter;
  sourceOptions: FilterMenuOption[];
  showSourceFilter: boolean;
  onSource: (k: SourceFilter) => void;
  bot: string;
  botOptions: FilterMenuOption[];
  showBotFilter: boolean;
  onBot: (k: string) => void;
  repo: string;
  repoOptions: FilterMenuOption[];
  showRepoFilter: boolean;
  mode: RunMode;
  onRepo: (k: string) => void;
  since: SinceFilter;
  onSince: (k: SinceFilter) => void;
  filtersActive: boolean;
  onClearFilters: () => void;
}) {
  return (
    <>
      <div className="flex-1 min-w-48 max-w-md">
        <Input
          type="search"
          value={queryInput}
          onChange={(e) => onQueryChange(e.currentTarget.value)}
          placeholder="Search name, workflow, file path, run id…"
          leadingIcon={<MagnifyingGlassIcon />}
          aria-label="Search runs"
        />
      </div>

      <FilterMenu
        axis="Status"
        ariaLabel="Filter by status"
        value={status}
        options={statusOptions}
        onSelect={(k) => onStatus(k as RunStatus | "")}
      />
      {showSourceFilter && (
        <FilterMenu
          axis="Source"
          ariaLabel="Filter by run source"
          value={source}
          options={sourceOptions}
          onSelect={(k) => onSource(k as SourceFilter)}
        />
      )}
      {showBotFilter && (
        <FilterMenu
          axis="Bot"
          ariaLabel="Filter by bot"
          value={bot}
          options={botOptions}
          onSelect={onBot}
        />
      )}
      {showRepoFilter && (
        <FilterMenu
          axis={repoAxisLabel(mode)}
          ariaLabel={`Filter by ${repoAxisLabel(mode).toLowerCase()}`}
          value={repo}
          options={repoOptions}
          onSelect={onRepo}
        />
      )}
      <FilterMenu
        axis="Since"
        ariaLabel="Filter by date"
        value={since}
        defaultValue="all"
        options={SINCE_FILTERS.map((f) => ({ key: f.value, label: f.label }))}
        onSelect={(k) => onSince(k as SinceFilter)}
      />
      {filtersActive && (
        <IconButton
          label="Clear filters"
          size="sm"
          variant="ghost"
          onClick={onClearFilters}
        >
          <Cross2Icon />
        </IconButton>
      )}
    </>
  );
}
