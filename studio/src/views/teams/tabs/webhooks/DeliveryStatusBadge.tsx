import { Badge } from "@/components/ui/Badge";

// Maps the spine's delivery status strings to a Badge variant so the
// deliveries drawer reads at a glance.
export function DeliveryStatusBadge({ status }: { status: string }) {
  const map: Record<string, "success" | "warning" | "danger" | "neutral"> = {
    launched: "success",
    accepted: "success",
    duplicate: "neutral",
    rate_limited: "warning",
    quota_exceeded: "warning",
    filtered: "neutral",
    invalid: "danger",
    launch_error: "danger",
  };
  const variant = map[status] ?? "neutral";
  return <Badge variant={variant}>{status}</Badge>;
}
