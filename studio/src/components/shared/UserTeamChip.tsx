import { useState } from "react";
import { CaretSortIcon } from "@radix-ui/react-icons";
import { useLocation } from "wouter";

import { useAuth } from "@/auth/AuthContext";
import { Popover, PopoverClose } from "@/components/ui/Popover";

// Hidden entirely in local dev mode (user id "dev"), where the desktop
// app's native menus drive Settings / ProjectSwitcher instead.
export default function UserTeamChip({ collapsed = false }: { collapsed?: boolean }) {
  const { user, teams, activeTeamID, activeTeam, signOut, selectTeam } = useAuth();
  const [, navigate] = useLocation();
  const [open, setOpen] = useState(false);

  const isLocal = user?.id === "dev";
  if (isLocal) return null;

  const teamLabel = activeTeam?.team_name ?? "No team";
  // Initials for the avatar: first letter of the first two words of the
  // team name (fallback to the user's email initial).
  const initials =
    teamLabel
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((w) => w[0])
      .join("")
      .toUpperCase() || (user?.email?.[0]?.toUpperCase() ?? "?");

  // PopoverClose wraps each menu button so the popover dismisses on
  // click — equivalent to the previous setOpen(false) lines but keyboard-
  // accessible (Radix wires Escape + focus return for us).
  const closeAfter = (fn: () => void) => () => {
    fn();
    setOpen(false);
  };

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      side="top"
      align="start"
      contentClassName="w-[min(18rem,calc(100vw-1rem))] p-2 text-sm"
      trigger={
        collapsed ? (
          <button
            type="button"
            className="inline-flex h-7 w-7 items-center justify-center rounded-full bg-accent text-fg-onAccent text-caption font-semibold uppercase hover:opacity-80 transition-opacity"
            title={`${teamLabel} · ${user?.email ?? ""}`}
            aria-label={`Account menu — ${teamLabel}, ${user?.email ?? ""}`}
          >
            {initials}
          </button>
        ) : (
          <button
            type="button"
            className="flex w-full items-center gap-2 rounded px-2 py-1 text-left hover:bg-surface-2 transition-colors"
            title={`${teamLabel} · ${user?.email ?? ""}`}
            aria-label={`Account menu — ${teamLabel}, ${user?.email ?? ""}`}
          >
            <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent text-fg-onAccent text-caption font-semibold uppercase">
              {initials}
            </span>
            <span className="min-w-0 flex-1 leading-tight">
              <span className="block truncate text-xs font-medium text-fg-default">
                {teamLabel}
              </span>
              {user?.email && (
                <span className="block truncate text-caption text-fg-muted">
                  {user.email}
                </span>
              )}
            </span>
            <CaretSortIcon className="h-4 w-4 shrink-0 text-fg-subtle" />
          </button>
        )
      }
    >
      <div className="px-2 py-1 text-xs uppercase tracking-wider text-fg-muted">
        Switch team
      </div>
      {teams.map((t) => (
        <PopoverClose asChild key={t.team_id}>
          <button
            onClick={closeAfter(() => void selectTeam(t.team_id))}
            className={`w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 ${
              t.team_id === activeTeamID ? "bg-surface-2" : ""
            }`}
          >
            <div className="font-medium">{t.team_name}</div>
            <div className="text-xs text-fg-muted">
              {t.role}
              {t.personal && " · personal"}
            </div>
          </button>
        </PopoverClose>
      ))}
      <div className="my-1 border-t border-border-subtle" />
      {activeTeam && (
        <PopoverClose asChild>
          <button
            onClick={closeAfter(() => navigate(`/teams/${activeTeam.team_id}`))}
            className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2"
          >
            Manage {activeTeam.team_name}
          </button>
        </PopoverClose>
      )}
      <PopoverClose asChild>
        <button
          onClick={closeAfter(() => navigate("/account"))}
          className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2"
        >
          Account settings
        </button>
      </PopoverClose>
      {user?.is_super_admin && (
        <>
          <PopoverClose asChild>
            <button
              onClick={closeAfter(() => navigate("/admin/orgs"))}
              className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 text-warning-fg"
            >
              Platform admin · Organizations
            </button>
          </PopoverClose>
          <PopoverClose asChild>
            <button
              onClick={closeAfter(() => navigate("/admin/users"))}
              className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 text-warning-fg"
            >
              Platform admin · Users
            </button>
          </PopoverClose>
        </>
      )}
      <PopoverClose asChild>
        <button
          onClick={closeAfter(() => void signOut())}
          className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 text-danger"
        >
          Sign out
        </button>
      </PopoverClose>
    </Popover>
  );
}
