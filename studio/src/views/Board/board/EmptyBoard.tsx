// EmptyBoard renders the "tracker not initialised on disk yet" guide.
// The "board exists but has no issues" case is handled by EmptyBoardBanner
// so the column headers stay visible.
export function EmptyBoard({ kind }: { kind: "missing" }) {
  if (kind === "missing") {
    return (
      <div className="p-8 max-w-lg mx-auto text-fg-default space-y-4">
        <div className="text-lg font-semibold">Native tracker not initialised</div>
        <p className="text-sm text-fg-muted">
          The board view persists issues under the project's{" "}
          <code className="text-xs bg-surface-2 px-1 rounded">.iterion/dispatcher/native/</code>{" "}
          directory. iterion creates one automatically on first launch.
        </p>
        <div className="text-sm">
          <p className="mb-1 text-fg-default">Start it from the workspace:</p>
          <pre className="bg-surface-2 rounded p-2 text-xs font-mono overflow-x-auto">
            iterion studio --dir &lt;your-project&gt;
          </pre>
        </div>
      </div>
    );
  }
  return null;
}
