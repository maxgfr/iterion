import { Spinner } from "@/components/ui/Spinner";

// Suspense fallback for the route-level lazy chunks. Rendered inside the
// persistent AppShell's <main>, so the sidebar (and any contextual
// header bar) stays mounted while the next view's chunk arrives.
export default function MainSpinner() {
  return (
    <div className="flex h-full w-full items-center justify-center text-fg-muted text-xs gap-2">
      <Spinner size="sm" label="Loading view" />
      <span>Loading…</span>
    </div>
  );
}
