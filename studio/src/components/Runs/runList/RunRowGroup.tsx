import type { ReactNode } from "react";

// RunRowGroup renders one group's rows inside the desktop <table>. When
// the active group key is "none", the header row is suppressed and
// only the rows pass through — keeps the table identical to the
// pre-feature layout.
export function RunRowGroup({
  label,
  count,
  showHeader,
  columnSpan,
  children,
}: {
  label: string;
  count: number;
  showHeader: boolean;
  columnSpan: number;
  children: ReactNode;
}) {
  if (!showHeader) {
    // Fragments in <tbody> render directly as a child sequence — no
    // wrapping element, so the rows keep their normal striping.
    return <>{children}</>;
  }
  return (
    <>
      <tr className="bg-surface-2 border-y border-border-default">
        <th
          colSpan={columnSpan}
          scope="rowgroup"
          className="text-left px-4 py-1.5 font-medium text-fg-muted text-micro uppercase tracking-wide"
        >
          <span>{label}</span>
          <span className="ml-2 text-fg-subtle normal-case tracking-normal">
            {count}
          </span>
        </th>
      </tr>
      {children}
    </>
  );
}
