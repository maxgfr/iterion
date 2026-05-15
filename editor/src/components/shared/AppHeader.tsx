import { Link } from "wouter";
import type { ReactNode } from "react";

import NavLinks, { type Section } from "./NavLinks";
import ProjectLabel from "./ProjectLabel";
import UserTeamChip from "./UserTeamChip";
import BackendStatusPill from "@/components/Toolbar/BackendStatusPill";
import ThemeToggle from "@/components/ui/ThemeToggle";

interface AppHeaderProps {
  active?: Section;
  children?: ReactNode;
  rightActions?: ReactNode;
  showBackendPill?: boolean;
}

export default function AppHeader({
  active,
  children,
  rightActions,
  showBackendPill = true,
}: AppHeaderProps) {
  return (
    <header className="shrink-0 h-12 flex items-center gap-3 px-4 text-sm bg-surface-1 border-b border-border-default">
      <Link
        href="/"
        className="font-bold tracking-wide text-sm hover:text-accent transition-colors"
        title="Iterion home"
      >
        ITERION
      </Link>
      <NavLinks active={active} />
      <ProjectLabel />
      {children}
      <div className="ml-auto flex items-center gap-2">
        {rightActions}
        {showBackendPill && <BackendStatusPill />}
        <ThemeToggle />
        <UserTeamChip />
      </div>
    </header>
  );
}
