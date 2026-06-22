import { useCallback, useEffect, useState } from "react";

import {
  getBoard,
  listIssues,
  type NativeBoard,
  type NativeIssue,
} from "@/api/native";

export interface UseBoardDataResult {
  board: NativeBoard | null;
  issues: NativeIssue[];
  setIssues: React.Dispatch<React.SetStateAction<NativeIssue[]>>;
  loading: boolean;
  error: string | null;
  setError: React.Dispatch<React.SetStateAction<string | null>>;
  refresh: () => Promise<void>;
}

// Owns the initial fetch of board + issues. Exposes a refresh()
// imperative so mutating callers (create / save / delete / bulk ops)
// can re-pull after their writes. `setIssues` and `setError` are
// surfaced so optimistic-update / failure paths (onDrop, polls) can
// patch state without going through the round-trip.
export function useBoardData(): UseBoardDataResult {
  const [board, setBoard] = useState<NativeBoard | null>(null);
  const [issues, setIssues] = useState<NativeIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [b, i] = await Promise.all([getBoard(), listIssues()]);
      setBoard(b);
      setIssues(i ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return {
    board,
    issues,
    setIssues,
    loading,
    error,
    setError,
    refresh,
  };
}
