export type SpinnerSize = "xs" | "sm" | "md";

export interface SpinnerProps {
  size?: SpinnerSize;
  className?: string;
  label?: string;
}

const sizeClass: Record<SpinnerSize, string> = {
  xs: "h-3 w-3 border",
  sm: "h-3.5 w-3.5 border-2",
  md: "h-5 w-5 border-2",
};

// Indeterminate spinner — single source of truth for the studio's
// "something's loading and we don't know for how long" state. Use
// `label` for screen readers when the spinner is the only feedback
// for a region (omit when there's a sibling visible text like
// "Loading…").
export function Spinner({ size = "sm", className = "", label }: SpinnerProps) {
  return (
    <span
      role={label ? "status" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={`inline-block rounded-full border-current border-t-transparent animate-spin ${sizeClass[size]} ${className}`.trim()}
    />
  );
}
