import { useCallback, useEffect, useRef, useState } from "react";

export interface UseFetchResourceOptions<T> {
  /** Skip the effect (e.g. param not ready). Re-enabling triggers a fetch. */
  enabled?: boolean;
  /** Seed value while the first fetch is in flight. */
  initialData?: T;
}

export interface UseFetchResourceResult<T> {
  data: T | undefined;
  loading: boolean;
  error: string | null;
  /** Manually re-run the fetcher. Returns the new value (or throws). */
  refresh: () => Promise<T>;
}

/**
 * Thin wrapper for the standard "fetch on mount + refresh on demand"
 * pattern. Use this for any simple async resource — REST list, host
 * bridge call, single-shot RPC. It handles:
 *
 *  - mount + key-change refetch (`deps` array)
 *  - latest-wins race guard so a stale response doesn't clobber a new one
 *  - a stable `refresh()` callback for manual reloads
 *  - consistent `{ data, loading, error }` shape so callers can pipe
 *    straight into `<EmptyState />` + `<Skeleton />` primitives
 *
 * **Don't use this for** polling, event-triggered refetch, fingerprint
 * dedup, or module-level cache sharing — those concerns belong to
 * dedicated hooks. See `useRuns` / `useRunFiles` / `useEffortCapabilities`
 * for examples that intentionally don't use this wrapper.
 *
 * Usage:
 *
 *   const { data, loading, error, refresh } = useFetchResource(
 *     () => desktop.getSecretStatuses(),
 *     [],
 *   );
 *
 * The `deps` array is the cache key: when its serialised shape changes,
 * the fetcher reruns. Pass the inputs that determine the result (path,
 * id, mode...) — not the fetcher's closures.
 */
export function useFetchResource<T>(
  fetcher: () => Promise<T>,
  deps: unknown[],
  options: UseFetchResourceOptions<T> = {},
): UseFetchResourceResult<T> {
  const { enabled = true, initialData } = options;
  const [data, setData] = useState<T | undefined>(initialData);
  const [loading, setLoading] = useState<boolean>(enabled);
  const [error, setError] = useState<string | null>(null);

  // Track the fetcher in a ref so refresh() always uses the latest
  // closure without re-running on every render.
  const fetcherRef = useRef(fetcher);
  useEffect(() => {
    fetcherRef.current = fetcher;
  });

  // Generation counter — every fresh invocation bumps it. A response
  // whose generation no longer matches is silently discarded.
  const genRef = useRef(0);

  const refresh = useCallback(async (): Promise<T> => {
    const myGen = ++genRef.current;
    setLoading(true);
    try {
      const result = await fetcherRef.current();
      if (myGen === genRef.current) {
        setData(result);
        setError(null);
        setLoading(false);
      }
      return result;
    } catch (e) {
      if (myGen === genRef.current) {
        setError(e instanceof Error ? e.message : String(e));
        setLoading(false);
      }
      throw e;
    }
  }, []);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    void refresh();
    // The deps array is opaque-by-design: callers pick the cache key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, refresh, ...deps]);

  useEffect(() => {
    // Bump the generation on unmount so any pending response is
    // discarded — prevents "setState on unmounted component" warnings
    // when the component remounts before the in-flight request settles.
    return () => {
      genRef.current += 1;
    };
  }, []);

  return { data, loading, error, refresh };
}
