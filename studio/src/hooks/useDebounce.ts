import { useEffect, useState } from "react";

// useDebounce returns a delayed echo of `value`. Identity-stable until
// the source value settles for `delayMs`, at which point the returned
// value updates. Use for filter inputs where re-running heavy work on
// every keystroke would be wasteful.
export function useDebounce<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}
