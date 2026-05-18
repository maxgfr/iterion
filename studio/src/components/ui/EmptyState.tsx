import type { ReactNode } from "react";

export interface EmptyStateProps {
  message: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
}

// Empty/loading placeholder shared across list/tree panels. Keeps the
// vocabulary (centered, fg-subtle, small body text) consistent so list
// surfaces feel like one product rather than ten ad-hoc placeholders.
// Pass an empty string to render the centered slot without any text —
// useful as a neutral pre-fetch slate before the first response arrives.
export function EmptyState({ message, icon, action, className = "" }: EmptyStateProps) {
  return (
    <div
      className={`flex h-full flex-col items-center justify-center gap-2 px-3 py-8 text-center text-xs text-fg-subtle ${className}`.trim()}
    >
      {icon && <span aria-hidden>{icon}</span>}
      {message !== "" && <div>{message}</div>}
      {action}
    </div>
  );
}
