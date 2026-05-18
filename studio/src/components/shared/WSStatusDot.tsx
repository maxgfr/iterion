interface Props {
  // Mirrors useRunStore.wsState — "open" | "connecting" | "reconnecting" |
  // "closed" plus any future values; rendered as a coloured dot with a
  // tooltip on hover so the live status stays unobtrusive.
  state: string;
}

interface Style {
  color: string;
  label: string;
  pulse: boolean;
}

function styleFor(state: string): Style {
  switch (state) {
    case "open":
      return { color: "bg-success", label: "Connected", pulse: false };
    case "connecting":
      return { color: "bg-info", label: "Connecting…", pulse: true };
    case "reconnecting":
      return { color: "bg-warning", label: "Reconnecting…", pulse: true };
    case "closed":
      return { color: "bg-danger", label: "Disconnected", pulse: false };
    default:
      return { color: "bg-surface-3", label: state || "Unknown", pulse: false };
  }
}

export default function WSStatusDot({ state }: Props) {
  const { color, label, pulse } = styleFor(state);
  return (
    <span
      className="inline-flex items-center"
      title={`WebSocket: ${label}`}
      aria-label={`WebSocket: ${label}`}
    >
      <span
        className={`inline-block w-2 h-2 rounded-full ${color} ${
          pulse ? "animate-pulse" : ""
        }`}
      />
    </span>
  );
}
