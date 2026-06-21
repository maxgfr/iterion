import { useEffect, useMemo, useState } from "react";

import { readJSONFlag, writeJSONFlag, removeFlag } from "@/lib/localStorageFlag";
import { useToggleSet } from "@/hooks/useToggleSet";

// Per-run filter persistence: `run-console.event-filters.v1.<runId>`.
// Schema is intentionally minimal so older entries can stay readable
// when fields grow; missing keys fall back to defaults.
interface PersistedFilters {
  search?: string;
  types?: string[];
}

function filterStorageKey(runId: string): string {
  return `run-console.event-filters.v1.${runId}`;
}

function loadPersistedFilters(runId: string): PersistedFilters | null {
  return normalizePersistedFilters(
    readJSONFlag<unknown>(filterStorageKey(runId), null),
  );
}

function normalizePersistedFilters(value: unknown): PersistedFilters | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  const out: PersistedFilters = {};
  if (typeof record.search === "string") out.search = record.search;
  if (Array.isArray(record.types)) {
    const types = record.types.filter((v): v is string => typeof v === "string");
    if (types.length > 0) out.types = types;
  }
  return out;
}

function savePersistedFilters(runId: string, value: PersistedFilters) {
  // Treat all-default as "delete entry" to keep storage clean. The common
  // case (no filter applied) writes nothing.
  if ((!value.search || value.search === "") && (!value.types || value.types.length === 0)) {
    removeFlag(filterStorageKey(runId));
    return;
  }
  writeJSONFlag(filterStorageKey(runId), value);
}

export interface UsePersistedEventFiltersResult {
  search: string;
  setSearch: (v: string) => void;
  activeTypes: ReadonlySet<string>;
  toggleActiveType: (t: string) => void;
  clearActiveTypes: () => void;
}

// Concentrates BOTH localStorage effects (load + save) AND the
// reset-on-runId effect (the set-state-in-effect cluster). Concentrating
// the runId-reset here is deliberate: a `key={runId}` remount would
// invalidate Virtuoso's cache and is NOT equivalent.
export function usePersistedEventFilters(
  runId: string | null | undefined,
): UsePersistedEventFiltersResult {
  // Lazy initial value: read localStorage once for first paint so the
  // chips and search box render with the persisted state from the
  // get-go. The effect below reloads when a parent reuses this
  // component for a different run.
  const initialPersisted = useMemo<PersistedFilters | null>(
    () => (runId ? loadPersistedFilters(runId) : null),
    // Initial paint only; runId changes are handled by the effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );
  const [search, setSearch] = useState(initialPersisted?.search ?? "");
  const {
    set: activeTypes,
    toggle: toggleActiveType,
    clear: clearActiveTypes,
    replace: replaceActiveTypes,
  } = useToggleSet<string>(initialPersisted?.types ?? []);
  const [filtersRunId, setFiltersRunId] = useState<string | null>(
    () => runId ?? null,
  );

  useEffect(() => {
    if (!runId) {
      setSearch("");
      clearActiveTypes();
      setFiltersRunId(null);
      return;
    }
    const next = loadPersistedFilters(runId);
    setSearch(next?.search ?? "");
    replaceActiveTypes(next?.types ?? []);
    setFiltersRunId(runId);
  }, [runId, clearActiveTypes, replaceActiveTypes]);

  // Persist on every change. We avoid debouncing the search input
  // because the writes are small and infrequent compared to typing
  // bursts in other inputs, and an immediate write means a hard reload
  // never loses keystrokes.
  useEffect(() => {
    if (!runId) return;
    if (filtersRunId !== runId) return;
    savePersistedFilters(runId, {
      search: search || undefined,
      types: activeTypes.size > 0 ? Array.from(activeTypes) : undefined,
    });
  }, [runId, filtersRunId, search, activeTypes]);

  return {
    search,
    setSearch,
    activeTypes,
    toggleActiveType,
    clearActiveTypes,
  };
}
