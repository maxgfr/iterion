import HumanChatTurn from "../HumanChatTurn";

// PendingTurnFooter wraps HumanChatTurn in a Claude-Code-style fixed
// footer: top border, slightly stronger surface, comfortable padding.
// The wrapped HumanChatTurn keeps all its existing rendering (form /
// free-text / actions) — only the surround changes.
export default function PendingTurnFooter({
  message,
  form,
  busy,
  contextPrefix,
  onSubmit,
}: {
  message: Parameters<typeof HumanChatTurn>[0]["message"];
  form: Parameters<typeof HumanChatTurn>[0]["form"];
  busy: boolean;
  contextPrefix?: string;
  onSubmit: Parameters<typeof HumanChatTurn>[0]["onSubmit"];
}) {
  return (
    <div
      className="border-t border-border-default bg-surface-1"
      role="status"
      aria-live="polite"
    >
      <div className="mx-auto max-w-3xl px-4 py-3">
        {contextPrefix && contextPrefix.length > 0 && (
          <div className="mb-2 text-micro text-fg-muted italic">
            {contextPrefix}
          </div>
        )}
        <HumanChatTurn
          message={message}
          form={form}
          busy={busy}
          onSubmit={onSubmit}
        />
      </div>
    </div>
  );
}
