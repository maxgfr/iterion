// Extracted from RunHeader.tsx to keep that file focused.
// Actionable hint banner shown below the header for failed/cancelled
// runs whose `run.error` code maps to a known recovery suggestion.

import type { RunHeader as RunHeaderType } from "@/api/runs";
import { Button } from "@/components/ui";
import { parseErrorCode, runErrorHint } from "@/lib/runErrorHints";

// ErrorHintRow recognises common RuntimeError codes embedded in the
// `run.error` field (the engine formats them as "[CODE] message …") and
// renders a small actionable hint banner below the header. Returns null
// when the run is healthy or the error code is not recognised — we
// intentionally stay quiet rather than show a generic "Try resuming"
// hint that would dilute the targeted ones.
export default function ErrorHintRow({
  run,
  onResume,
}: {
  run: RunHeaderType;
  onResume: () => void;
}) {
  if (!run.error) return null;
  if (
    run.status !== "failed" &&
    run.status !== "failed_resumable" &&
    run.status !== "cancelled"
  ) {
    return null;
  }
  const code = parseErrorCode(run.error);
  const hint = runErrorHint(code, run);
  if (!hint) return null;
  const canResume = run.status === "failed_resumable";
  return (
    <div className="shrink-0 px-4 py-2 bg-warning-soft/40 border-b border-border-default flex items-start gap-2 text-[11px]">
      <span className="font-medium text-warning-fg shrink-0">Hint:</span>
      <span className="text-fg-default flex-1">{hint}</span>
      {canResume && (
        <Button
          variant="primary"
          size="sm"
          onClick={onResume}
          className="shrink-0"
          title="Open the Resume dialog"
        >
          Resume…
        </Button>
      )}
    </div>
  );
}
