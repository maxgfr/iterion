import { Select } from "@/components/ui/Select";

import {
  SORT_OPTIONS,
  type GroupKey,
  type SortKey,
} from "../runListSortGroup";

// SortGroupControls renders the two compact <select> dropdowns that
// drive the client-side sort + grouping axes. Stateless: the parent
// owns the URL-synced state. Hidden on the tightest viewports — at
// that width the search box + chip strips already saturate the toolbar
// row, and the dropdowns wrap awkwardly.
export function SortGroupControls({
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
