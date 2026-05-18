import { useState } from "react";
import { useLocation } from "wouter";

import { useAuth } from "@/auth/AuthContext";

// Hidden entirely in local dev mode (user id "dev"), where the desktop
// app's native menus drive Settings / ProjectSwitcher instead.
export default function UserTeamChip() {
  const { user, teams, activeTeamID, activeTeam, signOut, selectTeam } = useAuth();
  const [, navigate] = useLocation();
  const [open, setOpen] = useState(false);

  const isLocal = user?.id === "dev";
  if (isLocal) return null;

  return (
    <div className="relative inline-flex">
      <button
        onClick={() => setOpen((v) => !v)}
        className="bg-surface-1/95 border border-border-subtle rounded px-3 py-1 text-xs flex items-center gap-2 shadow max-w-[160px] sm:max-w-none"
        title={`${activeTeam?.team_name ?? "No team"} · ${user?.email ?? ""}`}
      >
        <span className="font-medium truncate">{activeTeam?.team_name ?? "No team"}</span>
        {/* Hide email on narrow viewports — team name is enough chrome. */}
        <span className="hidden sm:inline text-fg-muted truncate">{user?.email}</span>
        <span>▾</span>
      </button>
      {open && (
        <div
          className="absolute right-0 top-full mt-1 w-72 bg-surface-1 border border-border-subtle rounded shadow-lg p-2 text-sm z-50"
          onMouseLeave={() => setOpen(false)}
        >
          <div className="px-2 py-1 text-xs uppercase tracking-wider text-fg-muted">
            Switch team
          </div>
          {teams.map((t) => (
            <button
              key={t.team_id}
              onClick={() => {
                void selectTeam(t.team_id);
                setOpen(false);
              }}
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
          ))}
          <div className="my-1 border-t border-border-subtle" />
          {activeTeam && (
            <button
              onClick={() => {
                navigate(`/teams/${activeTeam.team_id}`);
                setOpen(false);
              }}
              className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2"
            >
              Manage {activeTeam.team_name}
            </button>
          )}
          <button
            onClick={() => {
              navigate("/account");
              setOpen(false);
            }}
            className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2"
          >
            Account settings
          </button>
          {user?.is_super_admin && (
            <button
              onClick={() => {
                navigate("/admin");
                setOpen(false);
              }}
              className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 text-fg-warn"
            >
              Platform admin
            </button>
          )}
          <button
            onClick={() => void signOut()}
            className="w-full text-left px-2 py-1.5 rounded hover:bg-surface-2 text-fg-error"
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
