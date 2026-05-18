import type { ReactNode } from "react";

import AppHeader from "./AppHeader";
import type { Section } from "./NavLinks";

interface PageShellProps {
  active?: Section;
  headerChildren?: ReactNode;
  rightActions?: ReactNode;
  showBackendPill?: boolean;
  children: ReactNode;
}

export default function PageShell({
  active,
  headerChildren,
  rightActions,
  showBackendPill,
  children,
}: PageShellProps) {
  return (
    <div className="h-full flex flex-col overflow-hidden bg-surface-0 text-fg-default">
      <AppHeader
        active={active}
        rightActions={rightActions}
        showBackendPill={showBackendPill}
      >
        {headerChildren}
      </AppHeader>
      {children}
    </div>
  );
}
