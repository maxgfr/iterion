export interface BrandWordmarkProps {
  /** Collapsed form: a single tracked "I" monogram + caret (narrow sidebar). */
  compact?: boolean;
  className?: string;
}

/**
 * Iterion wordmark — tracked-out caps + a discreet accent caret (a terminal
 * cursor, the one gentle nod to hacker-culture in otherwise sober chrome).
 *
 * Pure text + a CSS bar, so it is crisp at any size and theme-perfect via
 * `currentColor` / `text-accent-text` — no rasterised favicon + `dark:invert`
 * crutch. The caret is intentionally static here (the blinking variant lives
 * in empty/loading affordances, not the always-visible logo).
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
      <span
        aria-hidden
        className="ml-0.5 inline-block h-[0.85em] w-0.5 bg-accent-text"
      />
    </span>
  );
}
