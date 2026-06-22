import { useCallback } from "react";

import {
  transitionIssue,
  type NativeIssue,
} from "@/api/native";

export interface UseBoardDragDropResult {
  onDrop: (
    issueID: string,
    toState: string,
    opts?: { recordHistory?: boolean },
  ) => Promise<boolean>;
  onColumnDrop: (ids: string[], toState: string) => void;
}

// Owns the optimistic-update + rollback path for a single-issue or
// bulk column drop. Receives `recordTransition` (from useUndoTransitions)
// as a param so the dragDrop↔undo dependency cycle is resolved in the
// orchestrator: useBoardDragDrop produces onDrop, which useUndoTransitions
// consumes, while the latter feeds recordTransition back here.
export function useBoardDragDrop({
  setIssues,
  setError,
  recordTransition,
}: {
  setIssues: React.Dispatch<React.SetStateAction<NativeIssue[]>>;
  setError: React.Dispatch<React.SetStateAction<string | null>>;
  recordTransition: (id: string, from: string) => void;
}): UseBoardDragDropResult {
  const onDrop = useCallback<UseBoardDragDropResult["onDrop"]>(
    async (issueID, toState, opts) => {
      const recordHistory = opts?.recordHistory ?? true;
      // Capture this invocation's pre-state in a per-call closure so
      // two near-simultaneous drops don't race over the same `before`
      // variable. The prior implementation hoisted `before` to the
      // outer scope and the second drop would overwrite the first
      // drop's snapshot before its async transitionIssue had a chance
      // to fail / roll back, restoring the wrong row.
      const draft: { snapshot: NativeIssue[]; prevState: string | null } = {
        snapshot: [],
        prevState: null,
      };
      setIssues((cur) => {
        draft.snapshot = cur;
        const found = cur.find((i) => i.id === issueID);
        draft.prevState = found?.state ?? null;
        return cur.map((i) => (i.id === issueID ? { ...i, state: toState } : i));
      });
      try {
        await transitionIssue(issueID, toState);
        if (recordHistory && draft.prevState && draft.prevState !== toState) {
          recordTransition(issueID, draft.prevState);
        }
        return true;
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        // Only revert this issue's row to its pre-drop state — leave
        // other concurrent edits in place. Falls back to full revert
        // when the row was reordered out of the snapshot.
        const previous = draft.snapshot;
        setIssues((cur) => {
          const prev = previous.find((i) => i.id === issueID);
          if (!prev) return previous;
          return cur.map((i) => (i.id === issueID ? prev : i));
        });
        // Report failure so batch callers (onBulkDispatch) can
        // distinguish a queued issue from one that never moved — a
        // swallowed transition error here previously let a bulk
        // "Dispatched N" success toast fire even when some issues
        // never reached the dispatch lane.
        return false;
      }
    },
    [setIssues, setError, recordTransition],
  );

  const onColumnDrop = useCallback(
    (ids: string[], toState: string) => {
      for (const id of ids) void onDrop(id, toState);
    },
    [onDrop],
  );

  return { onDrop, onColumnDrop };
}
