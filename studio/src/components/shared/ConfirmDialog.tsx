import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { Button } from "@/components/ui/Button";

interface SecondaryAction {
  label: string;
  onClick: () => void;
  variant?: "default" | "danger";
}

interface Props {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  confirmVariant?: "danger" | "default";
  onConfirm: () => void;
  onCancel: () => void;
  // Optional middle button — useful when the dialog has three real
  // outcomes ("Cancel" / "Do X" / "Do Y") rather than the default
  // confirm-or-cancel split. Sits between Cancel and the primary
  // action so the visual reading order is left→right destructive.
  secondaryAction?: SecondaryAction;
}

export default function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  confirmVariant = "default",
  onConfirm,
  onCancel,
  secondaryAction,
}: Props) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    // Remember what had focus, move focus to Cancel (least-destructive),
    // and restore it on close so keyboard users aren't dumped at <body>.
    previouslyFocused.current = document.activeElement as HTMLElement | null;
    cancelRef.current?.focus();
    return () => previouslyFocused.current?.focus?.();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onCancel();
        return;
      }
      if (e.key !== "Tab") return;
      // Focus trap: keep Tab / Shift+Tab cycling inside the dialog so
      // focus can't wander into the (visually inert) background DOM.
      const root = dialogRef.current;
      if (!root) return;
      const focusable = Array.from(
        root.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled"));
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!first || !last) return;
      const active = document.activeElement as HTMLElement | null;
      if (e.shiftKey && active === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      } else if (active && !root.contains(active)) {
        e.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onCancel]);

  if (!open) return null;

  // Strings render inside a <p> for the historical layout; ReactNode
  // bodies (multi-paragraph, inline strong, etc) render inside a div
  // so callers can supply their own structure.
  const messageNode =
    typeof message === "string" ? (
      <p className="text-xs text-fg-muted mb-4">{message}</p>
    ) : (
      <div className="text-xs text-fg-muted mb-4 space-y-2">{message}</div>
    );

  // Portal to document.body and pin z-[var(--z-confirm)] so the dialog
  // always stacks above a parent modal that opened it. The semantic
  // ladder lives in app.css @theme.
  const content = (
    <div className="fixed inset-0 z-[var(--z-confirm)] bg-black/50 flex items-center justify-center">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="bg-surface-1 border border-border-strong rounded-lg p-4 min-w-[300px] max-w-[440px]"
      >
        <h3 className="text-sm font-bold text-fg-default mb-2">{title}</h3>
        {messageNode}
        <div className="flex justify-end gap-2">
          <Button
            ref={cancelRef}
            variant="secondary"
            size="sm"
            onClick={onCancel}
          >
            Cancel
          </Button>
          {secondaryAction && (
            <Button
              variant={secondaryAction.variant === "danger" ? "danger" : "secondary"}
              size="sm"
              onClick={secondaryAction.onClick}
            >
              {secondaryAction.label}
            </Button>
          )}
          <Button
            variant={confirmVariant === "danger" ? "danger" : "primary"}
            size="sm"
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );

  if (typeof document === "undefined") return content;
  return createPortal(content, document.body);
}
