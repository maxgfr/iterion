import { type LabelHTMLAttributes, type ReactNode } from "react";
import { HelpHint } from "./HelpHint";

export interface FieldLabelProps extends LabelHTMLAttributes<HTMLLabelElement> {
  /** Optional help text shown as a `?` affordance after the label. */
  help?: string;
  /** id for the help span, so it can be referenced by `aria-describedby`. */
  helpId?: string;
  children: ReactNode;
}

/**
 * The canonical "block label above a control" used across the form panels.
 * Extracted from the form-field layer so every label shares one treatment
 * (`text-fg-subtle`, the `?` help affordance, the `htmlFor` association).
 */
export function FieldLabel({
  help,
  helpId,
  children,
  className = "",
  ...rest
}: FieldLabelProps) {
  return (
    <label className={`block text-xs text-fg-subtle mb-1 ${className}`.trim()} {...rest}>
      {children}
      {help && <HelpHint text={help} id={helpId} />}
    </label>
  );
}
