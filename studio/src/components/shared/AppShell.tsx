import { Suspense } from "react";
import type { ReactNode } from "react";

import Sidebar from "./Sidebar";
import ContextualHeaderBar from "./ContextualHeaderBar";
import MainSpinner from "./MainSpinner";
import { useUIStore } from "@/store/ui";

interface AppShellProps {
  children: ReactNode;
}

// AppShell is the persistent layout root for all authenticated routes.
// The sidebar + ContextualHeaderBar stay mounted across navigation;
// only <main> swaps its lazy-loaded route content.
//
// Focus mode: when useUIStore.expanded is true (editor canvas-only
// mode), the chrome is dropped entirely — including the "Skip to
// main content" link, since there's nothing to skip past.
export default function AppShell({ children }: AppShellProps) {
  const expanded = useUIStore((s) => s.expanded);

  if (expanded) {
    return (
      <div className="h-screen w-screen bg-surface-0 text-fg-default">
        <Suspense fallback={<MainSpinner />}>{children}</Suspense>
      </div>
    );
  }

  return (
    <div className="h-screen w-screen flex bg-surface-0 text-fg-default overflow-hidden">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[var(--z-toast)] focus:bg-accent focus:text-fg-onAccent focus:px-3 focus:py-1.5 focus:rounded focus:shadow-lg"
      >
        Skip to main content
      </a>
      <Sidebar />
      <div className="flex-1 min-w-0 flex flex-col">
        <ContextualHeaderBar />
        <main id="main-content" className="flex-1 min-h-0 overflow-hidden">
          <Suspense fallback={<MainSpinner />}>{children}</Suspense>
        </main>
      </div>
    </div>
  );
}
