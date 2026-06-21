// formatWallClock renders an ISO timestamp as HH:MM:SS in the user's
// locale so the duration cell can flip to absolute wall-clock anchors
// (R16). Falls back to the raw input when parsing fails — better to
// show something than silently lose the timestamp.
export function formatWallClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export type TabValue = "pause" | "trace" | "tools" | "artifact" | "events" | "logs";
