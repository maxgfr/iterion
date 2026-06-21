// useToggleSet replaces the hand-rolled `useState<Set<T>>` + add/remove
// toggle pattern repeated across filter strips, multi-select lists, and
// collapsible trees. Returns the Set + a stable toggle that creates a
// fresh Set on update (React's immutability requirement) plus a few
// extra setters (clear/replace/has) the call sites lean on.
import { useCallback, useState } from "react";

export interface ToggleSet<T> {
  set: Set<T>;
  has: (item: T) => boolean;
  toggle: (item: T) => void;
  clear: () => void;
  replace: (items: Iterable<T>) => void;
}

export function useToggleSet<T>(initial?: Iterable<T>): ToggleSet<T> {
  const [set, setSet] = useState<Set<T>>(() => new Set(initial));
  const toggle = useCallback((item: T) => {
    setSet((prev) => {
      const next = new Set(prev);
      if (next.has(item)) next.delete(item);
      else next.add(item);
      return next;
    });
  }, []);
  const clear = useCallback(() => {
    setSet(new Set());
  }, []);
  const replace = useCallback((items: Iterable<T>) => {
    setSet(new Set(items));
  }, []);
  const has = useCallback((item: T) => set.has(item), [set]);
  return { set, has, toggle, clear, replace };
}
