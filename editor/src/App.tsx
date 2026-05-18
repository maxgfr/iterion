import { Suspense, lazy, useEffect, useState } from "react";
import { Route, Switch } from "wouter";

// Routes are React.lazy'd so each view ships its own chunk and the
// initial download covers only the shell + AuthGate. The eager imports
// below are the always-needed shell pieces (Login lives off the auth
// gate; everything else is conditional on a route match).
const EditorView = lazy(() => import("@/components/EditorView"));
const HomeView = lazy(() => import("@/components/Home/HomeView"));
const PiloteView = lazy(() => import("@/components/Pilote/PiloteView"));
const LaunchView = lazy(() => import("@/components/Runs/LaunchView"));
const RunListView = lazy(() => import("@/components/Runs/RunListView"));
const RunView = lazy(() => import("@/components/Runs/RunView"));
const BoardView = lazy(() => import("@/views/Board"));
const ConductorView = lazy(() => import("@/views/Conductor"));
const Welcome = lazy(() => import("@/views/Welcome"));
const Settings = lazy(() => import("@/views/Settings"));
const ProjectSwitcher = lazy(() => import("@/views/ProjectSwitcher"));
const SettingsPage = lazy(() => import("@/views/settings/SettingsPage"));
const TeamPage = lazy(() => import("@/views/teams/TeamPage"));

import { ErrorBoundary } from "@/components/shared/ErrorBoundary";
import ToastContainer from "@/components/shared/Toast";
import MissingCLIBanner from "@/components/MissingCLIBanner";
import Login from "@/views/Login";
import { useDesktop } from "@/hooks/useDesktop";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { useProjectSwitchListener } from "@/hooks/useProjectSwitchListener";
import { onDesktopEvent } from "@/lib/desktopBridge";
import { DesktopEvent } from "@/lib/desktopEvents";
import { AuthProvider, useAuth } from "@/auth/AuthContext";
import { setUnauthorizedHandler } from "@/api/client";
import { useDocumentStore } from "@/store/document";
import { useServerInfoStore } from "@/store/serverInfo";

export default function App() {
  return (
    <AuthProvider>
      <AuthGate />
    </AuthProvider>
  );
}

// AuthGate decides between the Login view and the full editor based
// on the AuthProvider's status. It also wires the global 401
// interceptor so editor API calls bounce the user back to /login on
// session expiration.
function AuthGate() {
  const { status, signOut } = useAuth();

  useEffect(() => {
    setUnauthorizedHandler(() => {
      void signOut();
    });
    return () => setUnauthorizedHandler(null);
  }, [signOut]);

  if (status === "loading") {
    return (
      <div className="h-screen flex items-center justify-center bg-surface-0 text-fg-muted">
        Loading…
      </div>
    );
  }
  if (status === "anonymous") {
    return <Login />;
  }
  return <AuthedApp />;
}

function AuthedApp() {
  const { isDesktop, ready, firstRunPending, refresh, pickAndAddProject } =
    useDesktop();
  const serverInfo = useServerInfoStore((s) => s.info);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<string>("api-keys");
  const [switcherOpen, setSwitcherOpen] = useState(false);

  useDocumentTitle();
  // Reset SPA state on a server-side project hot-swap so the new
  // project's empty home view replaces whatever the user was looking
  // at. No-op in desktop (server-mode WS) and cloud modes.
  useProjectSwitchListener();

  useEffect(() => {
    const offs = [
      onDesktopEvent(DesktopEvent.MenuSettings, () => {
        setSettingsTab("api-keys");
        setSettingsOpen(true);
      }),
      onDesktopEvent(DesktopEvent.MenuSwitchProject, () => setSwitcherOpen(true)),
      // MenuNewProject opens the native directory picker directly —
      // previously it opened the switcher (same as MenuSwitchProject),
      // which forced users through an extra step. The picker is also
      // what the "+ Add project…" button inside the switcher uses.
      onDesktopEvent(DesktopEvent.MenuNewProject, () => {
        void pickAndAddProject();
      }),
      onDesktopEvent(DesktopEvent.MenuAbout, () => {
        setSettingsTab("about");
        setSettingsOpen(true);
      }),
      onDesktopEvent(DesktopEvent.MenuUndo, () => useDocumentStore.getState().undo()),
      onDesktopEvent(DesktopEvent.MenuRedo, () => useDocumentStore.getState().redo()),
    ];
    // Listen for the SPA-emitted open-switcher event from ProjectLabel
    // (clicking the project chip in the toolbar / run header).
    const onOpenSwitcher = () => setSwitcherOpen(true);
    window.addEventListener("iterion:open-project-switcher", onOpenSwitcher);
    return () => {
      offs.forEach((off) => off());
      window.removeEventListener("iterion:open-project-switcher", onOpenSwitcher);
    };
  }, [pickAndAddProject]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "p") {
        e.preventDefault();
        setSwitcherOpen(true);
      }
      if ((e.metaKey || e.ctrlKey) && e.key === ",") {
        e.preventDefault();
        setSettingsTab("api-keys");
        setSettingsOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  if (!ready) {
    return (
      <div className="h-screen bg-surface-0 text-fg-default p-8">Loading…</div>
    );
  }

  if (isDesktop && firstRunPending) {
    return (
      <Suspense
        fallback={
          <div className="h-screen bg-surface-0 text-fg-default p-8">Loading…</div>
        }
      >
        <Welcome onComplete={refresh} />
      </Suspense>
    );
  }

  return (
    <>
      {isDesktop && <MissingCLIBanner />}
      <Suspense
        fallback={
          <div className="h-screen flex items-center justify-center bg-surface-0 text-fg-muted">
            Loading view…
          </div>
        }
      >
        <Switch>
          <Route path="/runs/new">
            <ErrorBoundary area="Launch view">
              <LaunchView />
            </ErrorBoundary>
          </Route>
          <Route path="/runs/:id">
            {(params) => (
              <ErrorBoundary area="Run view" resetKey={params.id ?? null}>
                <RunView />
              </ErrorBoundary>
            )}
          </Route>
          <Route path="/runs">
            <ErrorBoundary area="Runs list">
              <RunListView />
            </ErrorBoundary>
          </Route>
          <Route path="/account" component={SettingsPage} />
          <Route path="/teams/:id" component={TeamPage} />
          {serverInfo?.native_tracker_enabled && (
            <Route path="/board">
              <ErrorBoundary area="Board view">
                <BoardView />
              </ErrorBoundary>
            </Route>
          )}
          {serverInfo?.conductor_enabled && (
            <Route path="/conductor">
              <ErrorBoundary area="Conductor view">
                <ConductorView />
              </ErrorBoundary>
            </Route>
          )}
          <Route path="/editor" component={EditorView} />
          <Route path="/pilote">
            <ErrorBoundary area="Pilote view">
              <PiloteView />
            </ErrorBoundary>
          </Route>
          <Route path="/" component={HomeView} />
          <Route component={HomeView} />
        </Switch>
      </Suspense>
      <ToastContainer />
      {/* Settings + ProjectSwitcher are also lazy and need their own
          Suspense boundary because they unmount/remount on open/close. */}
      <Suspense fallback={null}>
        {isDesktop && (
          <Settings
            open={settingsOpen}
            onClose={() => setSettingsOpen(false)}
            tab={settingsTab}
            onTabChange={setSettingsTab}
          />
        )}
        {/* ProjectSwitcher renders in both desktop and local-server modes.
            Cloud mode (no work_dir) renders nothing useful; we still mount
            it so the Ctrl+P shortcut and ProjectLabel chip have somewhere
            to dispatch — the dialog just shows an empty list there. */}
        {serverInfo?.mode !== "cloud" && (
          <ProjectSwitcher open={switcherOpen} onClose={() => setSwitcherOpen(false)} />
        )}
      </Suspense>
    </>
  );
}

