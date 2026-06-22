export interface BrandWordmarkProps {
  /** Collapsed form: a single tracked "I" monogram (narrow sidebar). */
  compact?: boolean;
  className?: string;
}

/**
 * Iterion wordmark — tracked-out caps. Pure text, so it stays crisp at any
 * size and theme-perfect via `currentColor` — no rasterised favicon +
 * `dark:invert` crutch. The graphical hexagon mark sits beside it in the
 * Sidebar; the wordmark deliberately carries no trailing caret (after the
 * tracked caps a vertical bar read as a stray "I" — "ITERIONI").
 */
export function BrandWordmark({ compact = false, className = "" }: BrandWordmarkProps) {
  return (
    <span className={`inline-flex items-center font-semibold text-fg-default select-none ${className}`.trim()}>
      <span className="sr-only">Iterion</span>
      <span
        aria-hidden
        className="uppercase tracking-[0.2em] text-title leading-none"
      >
        {compact ? "I" : "ITERION"}
      </span>
    </span>
  );
}
