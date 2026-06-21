// useCopyTimer encapsulates the "copied" feedback timer used by Copy
// buttons: trigger() sets a key as copied, schedules a reset after
// `delayMs`, and guarantees the pending timer is cleared on unmount so a
// stale callback can't fire on a removed component. Generic over the
// copied-key type (string for multi-target rows, undefined for booleans
// — pass `null` as the cleared state and the trigger key when copied).
import { useCallback, useEffect, useRef, useState } from "react";

export interface CopyTimer<K> {
  copied: K | null;
  trigger: (key: K) => void;
  reset: () => void;
}

export function useCopyTimer<K = true>(delayMs = 1500): CopyTimer<K> {
  const [copied, setCopied] = useState<K | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    return () => {
      if (timerRef.current != null) clearTimeout(timerRef.current);
    };
  }, []);
  const reset = useCallback(() => {
    if (timerRef.current != null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setCopied(null);
  }, []);
  const trigger = useCallback(
    (key: K) => {
      setCopied(key);
      if (timerRef.current != null) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        setCopied(null);
      }, delayMs);
    },
    [delayMs],
  );
  return { copied, trigger, reset };
}
