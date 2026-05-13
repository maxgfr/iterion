import LogLinesView from "./LogLinesView";

interface Props {
  runId: string;
  // Imperative log subscription wired by RunView. Passed through to
  // LogLinesView, which mounts the subscription. The hook ref-counts
  // so the bottom panel and the per-node Logs tab can coexist.
  subscribeLogs: (fromOffset?: number) => void;
  unsubscribeLogs: () => void;
  onCollapse?: () => void;
}

export default function RunLogPanel({
  runId,
  subscribeLogs,
  unsubscribeLogs,
  onCollapse,
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
    />
  );
}
