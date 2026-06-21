// Extracted from RunHeader.tsx to keep that file focused.
// Parent-run breadcrumb shown on a forked run; clicking opens the
// parent's tab.

import { useLocation } from "wouter";

import type { RunHeader as RunHeaderType } from "@/api/runs";
import { HeaderBanner } from "@/components/ui";

// ForkedFromRow surfaces the parent-run breadcrumb on a forked run.
// Renders a one-line "⑂ forked from <name> @ <node>/turn <N>" with
// a click handler that focuses the parent tab (opening it if absent).
// Only mounted when run.forked_from is set.
export default function ForkedFromRow({ run }: { run: RunHeaderType }) {
  const [, setLocation] = useLocation();
  const parentID = run.forked_from!;
  const anchor = run.fork_anchor;
  const nodeLabel = anchor?.node_id ?? "?";
  const turnLabel = anchor?.turn_index ?? -1;
  const focusParent = () => setLocation(`/runs/${encodeURIComponent(parentID)}`);
  return (
    <HeaderBanner tone="info">
      <span className="text-fg-muted">⑂ Forked from</span>
      <button
        onClick={focusParent}
        className="font-mono text-fg-default hover:text-info underline-offset-2 hover:underline"
        title="Open the parent run"
      >
        {parentID.slice(0, 12)}
      </button>
      <span className="text-fg-subtle">at</span>
      <span className="font-mono">{nodeLabel}</span>
      <span className="text-fg-subtle">/ turn</span>
      <span className="font-mono">{turnLabel}</span>
      {anchor?.rewind_code && (
        <span className="ml-1 rounded bg-warning-soft px-1 text-[10px] text-fg-default" title="Worktree was reset to the snapshot at this boundary">
          rewound
        </span>
      )}
    </HeaderBanner>
  );
}
