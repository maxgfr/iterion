// Shared formatting helpers used by both RunningTable and RetriesTable.
// Kept colocated with the Dispatcher view so the formatting stays scoped
// to this context (the rest of the studio uses its own relTime variants).

// relTime renders an ISO timestamp as a short human-relative label
// ("12s ago" / "in 3m"). Returns the raw input on un-parseable dates so
// the column never lies about an unknown timestamp.
export function relTime(iso: string): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return iso;
  const dt = (Date.now() - t) / 1000;
  if (dt < 0) {
    const abs = Math.abs(dt);
    if (abs < 60) return `in ${Math.round(abs)}s`;
    if (abs < 3600) return `in ${Math.round(abs / 60)}m`;
    return `in ${Math.round(abs / 3600)}h`;
  }
  if (dt < 60) return `${Math.round(dt)}s ago`;
  if (dt < 3600) return `${Math.round(dt / 60)}m ago`;
  return `${Math.round(dt / 3600)}h ago`;
}

// compactWorkspace trims the noisy `<store-root>/dispatcher/workspaces/`
// prefix common to all dispatcher-driven worktrees so the column reads
// as the issue's identifier suffix at a glance. Leaves arbitrary host
// paths intact (cloud runner pods, manually-launched runs) so the
// column never lies about what's on disk.
export function compactWorkspace(path: string): string {
  const m = path.match(/dispatcher\/workspaces\/(.+)$/);
  if (m && m[1]) return m[1];
  // Worktrees laid out by runtime.WithWorktree (not dispatcher) live
  // under `<store-root>/worktrees/<run-id>`; show only the run-id.
  const w = path.match(/\/worktrees\/(.+)$/);
  if (w && w[1]) return w[1];
  return path;
}
