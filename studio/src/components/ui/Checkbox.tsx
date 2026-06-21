import { forwardRef, useId, type InputHTMLAttributes, type ReactNode } from "react";

export interface CheckboxProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "size"> {
  /** Optional label rendered to the right; clicking it toggles the box. */
  label?: ReactNode;
  /** Optional `?` help affordance shown after the label. */
  help?: string;
}

/**
 * Native checkbox styled with design tokens. Stays a real `<input
 * type="checkbox">` so keyboard + screen-reader semantics come for free;
 * the brand accent colours the check via `accent-accent`. Focus styling is
 * the global `:focus-visible` outline (app.css) — no per-control ring.
 */
export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
  { label, help, className = "", id, ...rest },
  ref,
) {
  const autoId = useId();
  const inputId = id ?? autoId;
  const box = (
    <input
      ref={ref}
      id={inputId}
      type="checkbox"
      className={`h-3.5 w-3.5 shrink-0 rounded border-border-strong bg-surface-1 accent-accent disabled:opacity-60 disabled:cursor-not-allowed ${
        label ? "" : className
      }`.trim()}
      {...rest}
    />
  );
  if (!label) return box;
  return (
    <label
      htmlFor={inputId}
      className={`inline-flex items-center gap-2 text-xs text-fg-muted cursor-pointer select-none ${className}`.trim()}
    >
      {box}
      <span>
        {label}
        {help && (
          <span
            className="text-fg-subtle hover:text-fg-muted cursor-help ml-1"
            title={help}
            aria-label={help}
          >
            ?
          </span>
        )}
      </span>
    </label>
  );
});
