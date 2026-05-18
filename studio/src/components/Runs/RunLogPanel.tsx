import LogLinesView from "./LogLinesView";

interface Props {
  runId: string;
  // Imperative log subscription wired by RunView. Passed through to
  // LogLinesView, which mounts the subscription. The hook ref-counts
  // so the bottom panel and the per-node Logs tab can coexist.
  subscribeLogs: (fromOffset?: number) => void;
  unsubscribeLogs: () => void;
  onCollapse?: () => void;
  // Absolute byte offset to clamp the displayed log to. Wired by
  // RunView from events[scrubSeq].log_offset so the bottom log panel
  // rewinds in lockstep with the canvas + EventLog during scrub /
  // replay. Null means live (no clamp).
  clampToBytes?: number | null;
}

export default function RunLogPanel({
  runId,
  subscribeLogs,
  unsubscribeLogs,
  onCollapse,
  clampToBytes = null,
}: Props) {
  return (
    <LogLinesView
      runId={runId}
      subscribeLogs={subscribeLogs}
      unsubscribeLogs={unsubscribeLogs}
      filterNodeId={null}
      filterIteration={null}
      showTitle
      onCollapse={onCollapse}
      clampToBytes={clampToBytes}
    />
  );
}
