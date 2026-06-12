import { useQuery } from "@tanstack/react-query";

import { listRunRepos, type RunRepo } from "@/api/runs";

// Stable empty fallback so the undefined→loaded transition doesn't hand
// consumers a fresh [] reference each render.
const EMPTY: RunRepo[] = [];

export interface UseRunReposResult {
  repos: RunRepo[];
  loading: boolean;
  error: string | null;
}

// Fetches the distinct repositories (cloud project_path) that have runs,
// with counts — feeds the run-list "by repo" filter chips. Decoupled
// from the runs list itself so selecting a repo (which narrows the list)
// doesn't make the other repo chips vanish.
//
// Cloud-mode only: pass `enabled: false` in local/desktop mode (where
// runs carry no project_path and folder chips are derived client-side).
// Polls lazily — the repo set changes far slower than the runs list, so
// a 30s refetch keeps new repos appearing without hammering the server.
export function useRunRepos(enabled: boolean): UseRunReposResult {
  const query = useQuery<RunRepo[]>({
    queryKey: ["run-repos"],
    queryFn: () => listRunRepos(),
    enabled,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });

  return {
    repos: query.data ?? EMPTY,
    loading: query.isLoading,
    error: query.error ? (query.error as Error).message : null,
  };
}
