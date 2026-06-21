// useAsyncAction encapsulates the busy/error try/catch/finally that
// every "fire an API call from a button" site re-implements. `run(fn)`
// flips busy on, clears any prior error, awaits fn(), maps a thrown
// error to errorMessage(e), and flips busy off in the finally — so the
// button stays disabled exactly while the call is in flight and the
// caller never forgets to reset state.
import { useCallback, useState } from "react";

import { errorMessage } from "@/lib/errorHints";

export interface AsyncAction {
  busy: boolean;
  error: string | null;
  // Run the async function under busy/error management. Returns the
  // value the function returned (or undefined on a thrown error) so
  // callers that need the result can still await it.
  run: <T>(fn: () => Promise<T>) => Promise<T | undefined>;
  clearError: () => void;
  // Direct setter for callers that want to surface a pre-flight
  // validation error without invoking run().
  setError: (msg: string | null) => void;
}

export function useAsyncAction(): AsyncAction {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const clearError = useCallback(() => setError(null), []);
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    setError(null);
    try {
      return await fn();
    } catch (e) {
      setError(errorMessage(e));
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);
  return { busy, error, run, clearError, setError };
}
