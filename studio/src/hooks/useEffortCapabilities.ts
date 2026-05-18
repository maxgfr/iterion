import { useEffect, useState } from "react";
import {
  fetchEffortCapabilities,
  type EffortCapabilities,
} from "@/api/client";

// Module-level cache shared across hook instances. The matrix never
// changes during a session (claw-code-go's registry is built-in; codex
// has its own server-side cache), so a single fetch per (backend, model)
// pair is enough for the lifetime of the studio.
const cache = new Map<string, EffortCapabilities>();
const inflight = new Map<string, Promise<EffortCapabilities>>();

function cacheKey(backend: string, model: string): string {
  return `${backend} ${model}`;
}

// effortBackendKey routes an unset backend through "claw" — that's
// where iterion sends LLM calls when the workflow doesn't pick
// claude_code or codex. Used as the backend half of the
// /api/effort-capabilities key.
export function effortBackendKey(backend: string | undefined): string {
  return backend?.trim() || "claw";
}

export function getCachedEffortCapabilities(
  backend: string,
  model: string,
): EffortCapabilities | undefined {
  return cache.get(cacheKey(backend, model));
}

// fetchAndCacheEffortCapabilities returns a deduplicated promise that
// resolves to the capabilities for a (backend, model) pair, populating
// the shared cache. Multiple callers awaiting the same key share the
// same in-flight request.
export function fetchAndCacheEffortCapabilities(
  backend: string,
  model: string,
): Promise<EffortCapabilities> {
  const key = cacheKey(backend, model);
  const cached = cache.get(key);
  if (cached) return Promise.resolve(cached);
  let promise = inflight.get(key);
  if (!promise) {
    promise = fetchEffortCapabilities(backend, model).then((result) => {
      cache.set(key, result);
      return result;
    });
    inflight.set(key, promise);
    promise.finally(() => inflight.delete(key));
  }
  return promise;
}

export interface UseEffortCapabilitiesResult {
  // null while the first fetch is pending.
  capabilities: EffortCapabilities | null;
  loading: boolean;
  error: string | null;
}

// useEffortCapabilities returns the supported reasoning_effort levels
// for the given (backend, model) pair, fetched lazily from the server.
// Returns capabilities=null while loading; once loaded, the value is
// cached and returned synchronously on subsequent calls with the same
// arguments.
export function useEffortCapabilities(
  backend: string | undefined,
  model: string | undefined,
): UseEffortCapabilitiesResult {
  const key =
    backend && model ? cacheKey(backend, model) : null;

  const [capabilities, setCapabilities] = useState<EffortCapabilities | null>(
    () => (key ? (cache.get(key) ?? null) : null),
  );
  const [loading, setLoading] = useState<boolean>(() => key !== null && !cache.has(key));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!backend || !model) {
      setCapabilities(null);
      setLoading(false);
      setError(null);
      return;
    }

    const cached = getCachedEffortCapabilities(backend, model);
    if (cached) {
      setCapabilities(cached);
      setLoading(false);
      setError(null);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    fetchAndCacheEffortCapabilities(backend, model)
      .then((result) => {
        if (cancelled) return;
        setCapabilities(result);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [backend, model, key]);

  return { capabilities, loading, error };
}
