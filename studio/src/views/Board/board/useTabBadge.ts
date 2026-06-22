import { useEffect, useMemo } from "react";

import type { NativeBoard, NativeIssue } from "@/api/native";

// Mirror the eligible-state issue counts into the browser tab title so
// operators with the board pinned in a background tab see new ready /
// in-progress work without focusing it. Derived as a stable string first
// so the effect only runs when the rendered counts actually change —
// `byState` gets a fresh Map identity every render, which would otherwise
// rewrite document.title every 2s on every poll tick.
export function useTabBadge({
  board,
  byState,
}: {
  board: NativeBoard | null;
  byState: Map<string, NativeIssue[]>;
}): void {
  const tabBadge = useMemo(() => {
    if (!board) return null;
    const eligible = board.states.filter((s) => s.eligible);
    if (eligible.length === 0) return null;
    const parts: string[] = [];
    for (const s of eligible) {
      const count = (byState.get(s.name) ?? []).length;
      if (count > 0) parts.push(`${count} ${s.display ?? s.name}`);
    }
    return parts.length > 0 ? `(${parts.join(", ")})` : null;
  }, [board, byState]);

  useEffect(() => {
    if (!tabBadge) return;
    const prev = document.title;
    document.title = `${tabBadge} ${prev}`;
    return () => {
      document.title = prev;
    };
  }, [tabBadge]);
}
