// Extracted from RunHeader.tsx to keep that file focused.
// "From ticket" row shown when the run was dispatched by the native
// dispatcher; clicking opens /board with the source issue focused.

import { useLocation } from "wouter";

import type { RunHeader as RunHeaderType } from "@/api/runs";

// SourceTicketRow surfaces the originating kanban issue when the run
// was dispatched by the native dispatcher. Clicking opens /board with
// the issue focused so the operator can jump back to the ticket that
// triggered this run — answering "what does this run belong to?"
// without grepping the dispatcher logs.
export default function SourceTicketRow({
  source,
}: {
  source: NonNullable<RunHeaderType["source"]>;
}) {
  const [, setLocation] = useLocation();
  const issueID = source.issue_id!;
  // Title-led display. The "[#N]" prefix is a project convention
  // baked into emit_action's titles, so it's already visible without
  // us echoing the internal identifier (which on the native tracker
  // is an ugly "native:<short-uuid>" chip). The full identifier
  // survives in the tooltip + the navigation URL for operators who
  // need it.
  const title = (source.issue_title || "(untitled)").trim();
  const shortHandle = parseTicketHandle(title, source.issue_identifier);
  const focusIssue = () =>
    setLocation(`/board?focus=${encodeURIComponent(issueID)}`);
  return (
    <div className="shrink-0 px-4 py-1.5 bg-info-soft/40 border-b border-info/30 flex items-center gap-2 text-[11px]">
      <span className="text-fg-muted shrink-0">From ticket</span>
      <button
        onClick={focusIssue}
        className="inline-flex items-center gap-2 text-fg-default hover:text-info underline-offset-2 hover:underline truncate min-w-0"
        title={`Open issue ${source.issue_identifier || issueID} on the board`}
      >
        {shortHandle && (
          <span className="font-mono shrink-0 text-fg-muted">{shortHandle}</span>
        )}
        <span className="truncate text-fg-default">{title}</span>
      </button>
    </div>
  );
}

// parseTicketHandle returns the human-friendly handle the operator
// recognises: when emit_action prefixed the title with "[#N]" we lift
// that out as a separate mono chip; otherwise we render nothing and
// the navigation tooltip carries the long identifier. We intentionally
// don't synthesise a chip from the tracker's "native:<uuid-prefix>"
// identifier — that bare UUID is more noise than signal next to the
// title.
function parseTicketHandle(
  title: string,
  fallbackIdentifier: string | null | undefined,
): string | null {
  const match = title.match(/^\[(#[^\]]+)\]/);
  if (match) return match[1] ?? null;
  if (fallbackIdentifier && !fallbackIdentifier.includes(":")) {
    return `#${fallbackIdentifier}`;
  }
  return null;
}
