import { Button } from "@/components/ui/Button";

// ResumeFooter is the bottom-of-chat call-to-action when the run is
// in failed_resumable or cancelled state. It takes priority over the
// always-on composer so the operator's next action is obvious:
// re-enter the run from its last checkpoint.
export default function ResumeFooter({
  runStatus,
  busy,
  onResume,
}: {
  runStatus: "failed_resumable" | "cancelled";
  busy: boolean;
  onResume: () => void;
}) {
  const explainer =
    runStatus === "failed_resumable"
      ? "A step failed. The run kept its checkpoint — Resume retries from that point."
      : "Run was cancelled. Resume picks up from the last checkpoint.";
  return (
    <div className="border-t border-border-default bg-surface-1">
      <div className="mx-auto max-w-3xl px-4 py-3 flex items-center gap-3">
        <div className="flex-1 text-body text-fg-muted">{explainer}</div>
        <Button
          variant="primary"
          size="sm"
          disabled={busy}
          onClick={onResume}
        >
          {busy ? "…" : "Resume"}
        </Button>
      </div>
    </div>
  );
}
