import { useEffect, useMemo, useState } from "react";
import { useLocation, useSearch } from "wouter";

import type { RunStatus } from "@/api/runs";
import { useDebounce } from "@/hooks/useDebounce";

import {
  parseSince,
  parseSource,
  type SinceFilter,
  type SourceFilter,
} from "../runListFilter";
import {
  parseGroup,
  parseSort,
  type GroupKey,
  type SortKey,
} from "../runListSortGroup";

const QUERY_DEBOUNCE_MS = 150;

export interface UseRunListFiltersResult {
  // URL-synced filter state
  status: RunStatus | "";
  setStatus: (v: RunStatus | "") => void;
  queryInput: string;
  setQueryInput: (v: string) => void;
  query: string; // debounced
  since: SinceFilter;
  setSince: (v: SinceFilter) => void;
  source: SourceFilter;
  setSource: (v: SourceFilter) => void;
  bot: string;
  setBot: React.Dispatch<React.SetStateAction<string>>;
  repo: string;
  setRepo: (v: string) => void;
  sort: SortKey;
  setSort: (v: SortKey) => void;
  group: GroupKey;
  setGroup: (v: GroupKey) => void;
}

// Owns the URL-backed filter state for the run list. Initial values
// derive from the URL so reload + browser-back restore the user's
// filters; subsequent state changes flow URL-ward via an effect, with
// replace semantics so the back-button stack stays uncluttered.
export function useRunListFilters(): UseRunListFiltersResult {
  const [location, setLocation] = useLocation();
  const search = useSearch();

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

  return {
    status,
    setStatus,
    queryInput,
    setQueryInput,
    query,
    since,
    setSince,
    source,
    setSource,
    bot,
    setBot,
    repo,
    setRepo,
    sort,
    setSort,
    group,
    setGroup,
  };
}
