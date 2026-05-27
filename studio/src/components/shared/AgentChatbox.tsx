import AgentChatboxInline from "./AgentChatboxInline";

interface Props {
  runId: string;
  disabled?: boolean;
  maxVisible?: number;
  // When the parent already renders queued messages inline in its
  // transcript (e.g. WhatsNextView), pass `embedded` so the chatbox
  // suppresses its built-in queue list — otherwise the transcript and
  // the chatbox both surface the same messages and the operator sees
  // duplicates. RunView's FloatingChatPanel leaves this false because
  // the popup is the only surface showing the queue there.
  embedded?: boolean;
}

// AgentChatbox is the legacy banner-style chatbox: a full-width strip
// with a top border and a centered max-w-3xl content column. Still
// used by WhatsNextView's flush-bottom layout.
//
// New surfaces (FloatingChatPanel in RunView) should render
// `AgentChatboxInline` directly without this chrome.
export default function AgentChatbox({
  runId,
  disabled = false,
  maxVisible = 5,
  embedded = false,
}: Props) {
  return (
    <div className="border-t border-border-subtle bg-surface-1">
      <div className="mx-auto max-w-3xl px-4 py-2">
        <AgentChatboxInline
          runId={runId}
          disabled={disabled}
          maxVisible={maxVisible}
          embedded={embedded}
        />
      </div>
    </div>
  );
}
