export interface TerminalCaretProps {
  className?: string;
}

/**
 * A blinking terminal cursor — the one gentle nod to hacker/terminal
 * culture, used in empty/loading affordances (its static sibling lives in
 * BrandWordmark). Decorative (`aria-hidden`): an accent-text bar whose
 * blink is paused under `prefers-reduced-motion` by the global app.css
 * rule, degrading to a steady caret.
 */
export function TerminalCaret({ className = "" }: TerminalCaretProps) {
  return (
    <span
      aria-hidden
      className={`inline-block h-[1em] w-0.5 translate-y-px bg-accent-text animate-caret-blink ${className}`.trim()}
    />
  );
}
