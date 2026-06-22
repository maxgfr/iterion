import { labelForStatus } from "@/components/Runs/runStatusMeta";
import type { useWhatsNextSession } from "@/lib/whats-next/useWhatsNextSession";

export function humanStatus(
  hi: ReturnType<typeof useWhatsNextSession>["status"],
  raw: ReturnType<typeof useWhatsNextSession>["runStatus"],
): string {
  if (hi === "launching") return "Launching…";
  if (hi === "submitting") return "Submitting…";
  if (hi === "ended") {
    const label = raw ? labelForStatus(raw) : "unknown";
    return `Ended · ${label}`;
  }
  if (raw === "paused_waiting_human") return "Waiting for your reply";
  return "Active";
}
