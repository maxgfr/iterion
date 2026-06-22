import { useEffect, useState } from "react";

import { getRunWorkflow, type WireWorkflow } from "@/api/runs";
import { errorMessage } from "@/lib/errorHints";

// useWorkflowLoad fetches the IR for the run on mount + on runId change.
// Surfaces both the resolved workflow and a string error so the canvas
// can swap between loading / error / ready states without owning the
// fetch lifecycle itself.
export interface WorkflowLoadResult {
  wf: WireWorkflow | null;
  error: string | null;
}

export function useWorkflowLoad(runId: string): WorkflowLoadResult {
  const [wf, setWf] = useState<WireWorkflow | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setWf(null);
    setError(null);
    let cancelled = false;
    getRunWorkflow(runId)
      .then((w) => {
        if (cancelled) return;
        setWf(w);
      })
      .catch((e) => {
        if (cancelled) return;
        setError(errorMessage(e));
      });
    return () => {
      cancelled = true;
    };
  }, [runId]);

  return { wf, error };
}
