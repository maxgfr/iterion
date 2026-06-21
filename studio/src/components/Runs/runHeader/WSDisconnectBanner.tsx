// Extracted from RunHeader.tsx to keep that file focused.
// Stale-data banner shown after the run WebSocket has been closed for
// at least 2 seconds; offers a manual reconnect button.

import { useEffect, useState } from "react";

import { Button, LiveDot } from "@/components/ui";
import type { WsState } from "@/store/run";

// 2-second debounce on the visible banner avoids flicker during normal
// reconnect blips — anything shorter than that the user shouldn't see.
export default function WSDisconnectBanner({
  state,
  onReconnect,
}: {
  state: WsState;
  onReconnect: () => void;
}) {
  const [showStale, setShowStale] = useState(false);
  useEffect(() => {
    if (state !== "closed") {
      setShowStale(false);
      return;
    }
    const t = window.setTimeout(() => setShowStale(true), 2000);
    return () => window.clearTimeout(t);
  }, [state]);
  if (!showStale) return null;
  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      className="px-4 py-1.5 bg-warning-soft border-b border-warning/40 flex items-center gap-2 text-micro text-warning-fg"
    >
      <LiveDot tone="danger" size="sm" pulse={false} />
      <span>Live updates disconnected — data may be stale.</span>
      <Button variant="ghost" size="sm" className="ml-auto" onClick={onReconnect}>
        Reconnect
      </Button>
    </div>
  );
}
