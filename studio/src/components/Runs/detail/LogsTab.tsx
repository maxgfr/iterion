import LogLinesView from "../LogLinesView";

export function LogsTab({
  runId,
  subscribeLogs,
  unsubscribeLogs,
  filterNodeId,
  filterIteration,
  clampToBytes,
}: {
  runId: string;
  subscribeLogs: (fromOffset?: number) => void;
  unsubscribeLogs: () => void;
  filterNodeId: string;
  filterIteration: number;
  clampToBytes?: number | null;
}) {
  return (
    <LogLinesView
      runId={runId}
      subscribeLogs={subscribeLogs}
      unsubscribeLogs={unsubscribeLogs}
      filterNodeId={filterNodeId}
      filterIteration={filterIteration}
      showTitle={false}
      clampToBytes={clampToBytes}
    />
  );
}
