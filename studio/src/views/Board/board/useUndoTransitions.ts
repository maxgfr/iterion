import { useCallback, useEffect, useRef, type MutableRefObject } from "react";

export interface TransitionHistoryEntry {
  id: string;
  from: string;
}

export interface UseTransitionHistoryResult {
  recordTransition: (id: string, from: string) => void;
  historyRef: MutableRefObject<TransitionHistoryEntry[]>;
}

// Owns the recent-transition history (bounded at 10) used for Ctrl+Z.
// Split from the keyboard-effect half so the orchestrator can wire
// drag-drop (which needs recordTransition) BEFORE wiring the undo
// shortcut (which needs the resulting onDrop). The undo shortcut lives
// in useUndoKeyboardShortcut below and shares the same historyRef.
//
// History is bounded at 10 entries — the board's drag-undo intent is the
// immediate "oops, wrong column", not full session replay.
export function useTransitionHistory(): UseTransitionHistoryResult {
  const historyRef = useRef<TransitionHistoryEntry[]>([]);

  const recordTransition = useCallback((id: string, from: string) => {
    const hist = historyRef.current;
    hist.push({ id, from });
    if (hist.length > 10) hist.shift();
  }, []);

  return { recordTransition, historyRef };
}

// Wires Ctrl/Cmd+Z to restore the most recent transition. Skips when a
// modal owns focus or an input is active so we don't fight form fields.
// The actual restore call goes through `onDrop` to keep the rollback
// path identical to the forward drag-drop one.
export function useUndoKeyboardShortcut({
  historyRef,
  onDrop,
  modalOpen,
}: {
  historyRef: MutableRefObject<TransitionHistoryEntry[]>;
  onDrop: (id: string, toState: string, opts?: { recordHistory?: boolean }) => Promise<boolean>;
  modalOpen: boolean;
}): void {
  const undoLastTransition = useCallback(() => {
    const last = historyRef.current.pop();
    if (!last) return;
    // Avoid re-recording the undo as a new history entry — otherwise
    // the user would just toggle between two columns forever.
    void onDrop(last.id, last.from, { recordHistory: false });
  }, [historyRef, onDrop]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.key !== "z" || e.shiftKey) return;
      const target = e.target as HTMLElement | null;
      const inInput =
        target?.tagName === "INPUT" ||
        target?.tagName === "TEXTAREA" ||
        target?.isContentEditable;
      if (inInput) return;
      if (modalOpen) return;
      if (historyRef.current.length === 0) return;
      e.preventDefault();
      undoLastTransition();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [historyRef, modalOpen, undoLastTransition]);
}
