export interface HelpHintProps {
  /** Tooltip text, surfaced as both `title` and `aria-label`. */
  text: string;
  /** Optional id so the hint can be referenced by `aria-describedby`. */
  id?: string;
}

/**
 * The "?" help affordance shared by FieldLabel + Checkbox (and the form
 * panels) — a cursor-help glyph carrying its text as title + aria-label.
 * Extracted so the markup lives in one place instead of drifting per copy.
 */
export function HelpHint({ text, id }: HelpHintProps) {
  return (
    <span
      id={id}
      className="text-fg-subtle hover:text-fg-muted cursor-help ml-1"
      title={text}
      aria-label={text}
    >
      ?
    </span>
  );
}
