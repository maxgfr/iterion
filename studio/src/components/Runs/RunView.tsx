import { errorMessage } from "@/lib/errorHints";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ReactFlowProvider } from "@xyflow/react";
import { useParams } from "wouter";
import { Group, Panel } from "react-resizable-panels";

import {
  getRun,
  type ExecutionState,
  type RunFile,
  type RunFilesMode,
} from "@/api/runs";
import { Tabs } from "@/components/ui";
import { selectRunningExecution, useRunStore } from "@/store/run";
import { useUIStore } from "@/store/ui";
import { useRunWebSocket } from "@/hooks/useRunWebSocket";
import { useLayoutPersistence } from "@/hooks/useLayoutPersistence";
import { useRunToasts } from "@/hooks/useRunToasts";
import { useRunKeyboard } from "@/hooks/useRunKeyboard";
import { writeBooleanFlag, writeStringFlag } from "@/lib/localStorageFlag";

import { buildExecutionsAt } from "@/lib/snapshotReducer";

import BrowserPane from "./BrowserPane";
import EventLog from "./EventLog";
import FileDiffDialog from "./FileDiffDialog";
import FileEditDialog from "./FileEditDialog";
import FloatingChatPanel, { ChatPanelContent } from "./FloatingChatPanel";
import OperatorPauseBanner from "./OperatorPauseBanner";
import LeftPanel from "./LeftPanel";
import NodeDetailPanel from "./NodeDetailPanel";
import QueuedBanner from "./QueuedBanner";
import RunCanvasIR, { defaultIterationFor } from "./RunCanvasIR";
import RunHeader from "./RunHeader";
import RunLogPanel from "./RunLogPanel";
import RunMetrics from "./RunMetrics";
import ArtifactFilesPanel from "./ArtifactFilesPanel";
import ReportTab from "./ReportTab";
import Scrubber from "./Scrubber";
import { ExpandStrip, ResizeSeparator } from "./runView/PanelChrome";
import { RunViewLoadError, RunViewSkeleton } from "./runView/RunViewLoadStates";
import {
  BOTTOM_TABS,
  BOTTOM_TAB_KEY,
  BOTTOM_TAB_LABELS,
  EVENTLOG_COLLAPSED_KEY,
  type BottomTab,
} from "./runView/layoutFlags";
import { useDisplayedRunData } from "./runView/useDisplayedRunData";
import { useRunConsoleLayout } from "./runView/useRunConsoleLayout";

interface RunViewProps {
  // Passed by RunTabHost when this view is hosted in a tab subtree.
  // Falls back to wouter's :id param so legacy `/runs/:id` deep links
  // still work when no host is in scope (e.g. the LaunchView preview).
  runId?: string | null;
}

export default function RunView({ runId: runIdProp }: RunViewProps = {}) {
  const params = useParams<{ id: string }>();
  const runId = runIdProp ?? params.id ?? null;

  const setRunId = useRunStore((s) => s.setRunId);
  const reset = useRunStore((s) => s.reset);
  const applySnapshot = useRunStore((s) => s.applySnapshot);
  const loadEventHistoryIfMissing = useRunStore((s) => s.loadEventHistoryIfMissing);
  const snapshot = useRunStore((s) => s.snapshot);
  const events = useRunStore((s) => s.events);
  const executionsById = useRunStore((s) => s.executionsById);
  const wsState = useRunStore((s) => s.wsState);
  const followTail = useRunStore((s) => s.followTail);
  const setFollowTail = useRunStore((s) => s.setFollowTail);
  const browserPreview = useRunStore((s) => s.browser);
  const browserAvailable =
    browserPreview.currentUrl !== null ||
    browserPreview.lastEventSeqSeen !== null ||
    browserPreview.screenshots.length > 0 ||
    browserPreview.liveSession !== null;
  // Time-travel scrubber: when non-null, the canvas/detail/event log
  // render the run *as it was* at this seq. When null (the default),
  // live data flows through. Lives in component state because it's
  // purely UI-driven; the store remains the source of truth for live.
  const [scrubSeq, setScrubSeq] = useState<number | null>(null);
  // Tracks whether the initial snapshot fetch has exhausted its retries
  // without success. Flipped true so the skeleton swaps for a clear
  // "Run not found" message instead of pulsing forever. Distinguishes
  // "loading" (snapshot null + !loadFailed) from "no such run on this
  // daemon" (snapshot null + loadFailed). Reset on runId change.
  const [loadFailed, setLoadFailed] = useState<{ status: number; message: string } | null>(null);
  // Workflow view selects by IR node id (not execution id). The detail
  // panel is driven by the selected node's currently-picked iteration
  // (per-node, default = "current"). Manual picks live alongside a
  // "follow live" toggle: when the toggle is on, the panel auto-shifts
  // to whatever node is currently running; when the user clicks a node
  // the toggle flips off so their pick stays pinned.
  const [manualSelectedNodeId, setManualSelectedNodeId] = useState<string | null>(null);
  const [followLiveNode, setFollowLiveNode] = useState<boolean>(true);
  // Per-IR-node iteration override. Empty map means "use the default
  // (running > paused > latest)". A user click on a timeline pip sets
  // the entry; we never auto-clear so the user's pick stays sticky.
  const [iterationByNode, setIterationByNode] = useState<Map<string, number>>(
    () => new Map(),
  );
  const handleSelectIteration = useCallback((nodeId: string, iteration: number) => {
    setIterationByNode((prev) => {
      const next = new Map(prev);
      next.set(nodeId, iteration);
      return next;
    });
  }, []);

  const handleSelectNode = useCallback((nodeId: string | null) => {
    setManualSelectedNodeId(nodeId);
    // Click on a real node pins it (and disables follow-live).
    // Click on empty pane / toggle-off does the opposite: re-engage
    // auto-follow so the user has an obvious "go back to live" path
    // without hunting for the FollowLivePill inside the detail panel.
    setFollowLiveNode(nodeId === null);
  }, []);

  const handleJumpToFailed = useCallback((nodeId: string) => {
    handleSelectNode(nodeId);
  }, [handleSelectNode]);

  const handleEventSelect = useCallback(
    (nodeId: string, iteration: number) => {
      handleSelectNode(nodeId);
      setIterationByNode((prev) => {
        const next = new Map(prev);
        next.set(nodeId, iteration);
        return next;
      });
    },
    [handleSelectNode],
  );

  const handleClearSelection = useCallback(() => {
    setManualSelectedNodeId(null);
  }, []);

  const handleToggleFollowLive = useCallback(() => {
    setFollowLiveNode((prev) => {
      const next = !prev;
      // Re-engaging follow live: drop the manual pin so the panel
      // jumps to the currently-running node on the next render.
      if (next) setManualSelectedNodeId(null);
      return next;
    });
  }, []);

  const [diffFile, setDiffFile] = useState<RunFile | null>(null);
  // Mode the FilesPanel was in when the user clicked the row; forwarded
  // to FileDiffDialog so it requests the same range from the backend.
  const [diffMode, setDiffMode] = useState<RunFilesMode>("");
  const handleSelectFile = useCallback(
    (file: RunFile, mode: RunFilesMode) => {
      setDiffMode(mode);
      setDiffFile(file);
    },
    [],
  );

  // Worktree path open in the editable Monaco tab (FileEditDialog), or null.
  // Driven by the FilesPanel "Edit .gitignore" shortcut and the diff
  // dialog's "Edit" affordance (which closes the read-only diff first).
  const [editFile, setEditFile] = useState<string | null>(null);
  const handleEditFile = useCallback((path: string) => {
    setDiffFile(null);
    setEditFile(path);
  }, []);

  const verticalLayout = useLayoutPersistence("run-console-v2.vertical", {
    top: 70,
    eventlog: 30,
  });
  const horizontalLayout = useLayoutPersistence(
    "run-console-v2.horizontal",
    { canvas: 70, detail: 30 },
  );
  // Separate layout key so the right-dock split (canvas / detail /
  // browser) doesn't collide with the canvas/detail-only layout when
  // the user toggles the dock.
  const horizontalLayoutWithBrowser = useLayoutPersistence(
    "run-console-v2.horizontal-with-browser",
    { canvas: 50, detail: 25, browserRight: 25 },
  );
  // Layout key for when the chat panel is docked to the right (3rd
  // resizable column). Includes the chat slot at the end.
  const horizontalLayoutWithChat = useLayoutPersistence(
    "run-console-v2.horizontal-with-chat",
    { canvas: 50, detail: 20, chat: 30 },
  );
  // Layout key for the rare case where both browser AND chat dock to
  // the right at the same time (4 horizontal columns).
  const horizontalLayoutWithBrowserAndChat = useLayoutPersistence(
    "run-console-v2.horizontal-full-right",
    { canvas: 40, detail: 20, browserRight: 20, chat: 20 },
  );

  // Persisted run-console layout/dock dials. The cross-cutting effects
  // below (auto-reveal Browser, "Show event log" token, browserDock →
  // bottomTab redirect) drive the raw setters this hook exposes.
  const {
    browserDock,
    setBrowserDock,
    detailCollapsed,
    toggleDetailCollapsed,
    eventlogCollapsed,
    toggleEventlogCollapsed,
    setEventlogCollapsed,
    bottomTab,
    setBottomTab,
    handleSetBottomTab,
    bottomTabPinned,
    setBottomTabPinned,
    chatDock,
    setChatDock,
  } = useRunConsoleLayout();

  // ConversationEmptyState's "Show event log" link bumps a token on
  // the run store; we expand the bottom drawer + flip to "events"
  // when the token changes. The token is a global per-store counter
  // (not reset by reset()), so we baseline against the value the
  // current run-view instance last saw — re-baselined on runId change
  // so navigating between run tabs doesn't replay a stale bump.
  const uiOpenEventLogToken = useRunStore((s) => s.uiOpenEventLogToken);
  const eventLogTokenBaselineRef = useRef(uiOpenEventLogToken);
  useEffect(() => {
    eventLogTokenBaselineRef.current = uiOpenEventLogToken;
    // Only re-baseline on runId change; ignore live token bumps here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId]);
  useEffect(() => {
    if (uiOpenEventLogToken === eventLogTokenBaselineRef.current) return;
    setBottomTab("events");
    writeStringFlag(BOTTOM_TAB_KEY, "events");
    setBottomTabPinned(true);
    setEventlogCollapsed((prev) => {
      if (!prev) return prev;
      writeBooleanFlag(EVENTLOG_COLLAPSED_KEY, false);
      return false;
    });
  }, [uiOpenEventLogToken, setBottomTab, setBottomTabPinned, setEventlogCollapsed]);

  // Auto-reveal the Browser tab the first time a preview URL becomes
  // available, but only if the user hasn't already pinned a different
  // tab during this view AND the user hasn't moved the pane to the
  // right side.
  useEffect(() => {
    if (
      !bottomTabPinned &&
      browserAvailable &&
      bottomTab !== "browser" &&
      browserDock === "bottom"
    ) {
      setBottomTab("browser");
    }
  }, [browserAvailable, bottomTab, bottomTabPinned, browserDock, setBottomTab]);

  // If the user moves the browser pane to the right side while it was
  // the active bottom tab, redirect them to "logs" so the bottom panel
  // doesn't render an empty/duplicated pane.
  useEffect(() => {
    if (browserDock === "right" && bottomTab === "browser") {
      setBottomTab("logs");
    }
  }, [browserDock, bottomTab, setBottomTab]);

  useEffect(() => {
    setRunId(runId);
    return () => reset();
  }, [runId, setRunId, reset]);

  // Hydrate the persisted event log eagerly on run open. RunMetrics
  // (always-visible header strip) folds cost + llm_step counts from
  // the events array, and ReportTab does the same for the cost
  // breakdowns — both render the empty state when no events are
  // loaded. Earlier lazy-load (gated on bottomTab === "events" ||
  // scrubSeq !== null) saved history-fetch time but hid those
  // header-level metrics until the user opened the Events tab. The
  // action dedupes per run via historyFetchedForRun, so this stays
  // cheap on re-renders and tab toggles.
  // On failure, surface a *persistent* toast with a Retry action so the
  // operator can re-attempt in place instead of having to close and
  // re-open the run. loadEventHistoryIfMissing rolls back its
  // historyFetchedForRun marker on failure, so re-invoking it here
  // genuinely retries the fetch.
  const loadHistory = useCallback(() => {
    if (!runId) return;
    loadEventHistoryIfMissing(runId).catch((err) => {
      console.warn("[run] event history hydration failed:", err);
      const msg = errorMessage(err);
      useUIStore.getState().addToast(
        `Couldn't load event history: ${msg}`,
        "error",
        { persistent: true, action: { label: "Retry", onClick: () => loadHistory() } },
      );
    });
  }, [runId, loadEventHistoryIfMissing]);
  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  // Initial snapshot via REST so the page renders immediately even if
  // the WS is still connecting; the hook's `applySnapshot` on connect
  // will replace it.
  //
  // Retry on 404: the launch API returns the run_id as soon as the
  // engine goroutine is scheduled, but the goroutine still needs a
  // beat to call store.CreateRun before run.json exists on disk.
  // Fetching too early therefore 404s, and without a retry the page
  // gets stuck in <RunViewSkeleton/> until the user reloads — the
  // WS path was supposed to fill the gap but doesn't always push the
  // initial snapshot eagerly. A short backoff loop closes the race
  // for the common case (run.json typically lands within ~50–200ms)
  // without papering over a genuinely missing run.
  useEffect(() => {
    if (!runId) return;
    let cancelled = false;
    let attempt = 0;
    let timerId: ReturnType<typeof setTimeout> | null = null;
    // Reset failure state on every runId change so a navigation to a
    // different (valid) run rehydrates cleanly after a prior 404.
    setLoadFailed(null);
    const fetchWithRetry = () => {
      getRun(runId)
        .then((snap) => {
          if (!cancelled) applySnapshot(snap);
        })
        .catch((err: Error) => {
          if (cancelled) return;
          attempt += 1;
          const msg = err?.message ?? "";
          const is404 = msg.includes("API error 404");
          const cap = is404 ? 3 : 20;
          if (attempt < cap) {
            // Track the timer so the cleanup can cancel it. The
            // prior implementation only flipped `cancelled` for the
            // setState path; the timer kept firing for the full
            // retry budget after navigation, hammering the network.
            timerId = setTimeout(() => {
              timerId = null;
              if (!cancelled) fetchWithRetry();
            }, 250);
          } else if (!cancelled) {
            setLoadFailed({ status: is404 ? 404 : 0, message: msg });
          }
        });
    };
    fetchWithRetry();
    return () => {
      cancelled = true;
      if (timerId != null) {
        clearTimeout(timerId);
        timerId = null;
      }
    };
  }, [runId, applySnapshot]);

  // refreshSnapshot — used by post-merge UI to refetch run.json so
  // RunHeader and the merge-state-driven UI catch up after a Commits-
  // tab merge action lands. The WS pushes events but not run-meta
  // updates, so a manual REST fetch is the simplest path.
  const refreshSnapshot = useCallback(() => {
    if (!runId) return;
    getRun(runId)
      .then(applySnapshot)
      .catch(() => undefined);
  }, [runId, applySnapshot]);

  const wsHandle = useRunWebSocket(runId);
  useRunToasts(events, snapshot?.last_seq);

  const liveExecutions = useMemo(
    () => Array.from(executionsById.values()),
    [executionsById],
  );
  const firstFailedNodeId = useMemo(() => {
    for (const ex of liveExecutions) {
      if (ex.status === "failed") return ex.ir_node_id;
    }
    return null;
  }, [liveExecutions]);

  // When follow-live is on, override the manual pick with the
  // currently-running execution. While scrubbing the timeline we
  // disable the auto-track so the panel reflects the past, not the
  // live tail.
  const runningExec = useMemo(() => {
    if (scrubSeq !== null) return null;
    return selectRunningExecution(executionsById);
  }, [scrubSeq, executionsById]);

  // Sticky follow-live cache: bridges the transient gap between
  // `node_finished` (previous exec flips to finished) and `node_started`
  // (next exec arrives). Those events are emitted by the engine across
  // separate WS messages — with a `SaveCheckpoint` disk I/O + edge
  // selection in between — so the client sees a brief window where no
  // execution carries status="running" even though the run is still
  // active and producing logs.
  //
  // Without this cache, `runningExec` flips to null during the window,
  // `wfSelectedNodeId` collapses to `manualSelectedNodeId` (null when
  // follow-live is engaged), and the detail panel + per-node log
  // filter both blank out until the next node_started lands. With it,
  // we hold the last known running node id so the UI stays anchored
  // through the gap and only updates once the new running exec
  // materialises.
  const [lastRunningNodeId, setLastRunningNodeId] = useState<string | null>(
    null,
  );
  useEffect(() => {
    if (runningExec) {
      setLastRunningNodeId(runningExec.ir_node_id);
    }
  }, [runningExec]);
  // Clear the cache when the run reaches a terminal state so we don't
  // keep showing a stale "live" node after finish/fail/cancel. Paused
  // intentionally keeps the cached node — the user is mid-interaction.
  useEffect(() => {
    const status = snapshot?.run?.status;
    if (
      status === "finished" ||
      status === "failed" ||
      status === "failed_resumable" ||
      status === "cancelled"
    ) {
      setLastRunningNodeId(null);
    }
  }, [snapshot?.run?.status]);

  // Per-run component-local state outlives the store's reset() (which
  // only nukes the zustand store), so navigating run A → run B would
  // otherwise drag scrub position, selected node, pinned iterations,
  // the diff dialog, the bottom-tab pin, and the sticky last-running
  // node id into the new run — producing empty/truncated timelines,
  // "ghost" node selections (when B's IR doesn't contain A's nodes),
  // and an unexpectedly pinned bottom tab. Layout/dock preferences
  // persisted to localStorage are intentionally left alone.
  useEffect(() => {
    setScrubSeq(null);
    setManualSelectedNodeId(null);
    setFollowLiveNode(true);
    setIterationByNode(new Map());
    setLastRunningNodeId(null);
    setDiffFile(null);
    setDiffMode("");
    setBottomTabPinned(false);
  }, [runId]);

  // Live-follow node id with sticky fallback. When `runningExec` is
  // non-null we always use its node id (truth). When it's null but the
  // run is still active, we fall back to `lastRunningNodeId` — typically
  // the just-finished node — so the follow-live UI doesn't blank out
  // mid-transition. When scrubbing/replaying, derive the running node
  // from the historical exec map at scrubSeq so the canvas focus
  // advances with the timeline instead of staying stuck on the user's
  // last manual pick.
  const followLiveNodeId = useMemo(() => {
    if (scrubSeq !== null) {
      const execs = buildExecutionsAt(events, scrubSeq);
      let best: ExecutionState | null = null;
      for (const e of execs) {
        if (e.status !== "running") continue;
        if (!best || (e.started_at ?? "") > (best.started_at ?? "")) {
          best = e;
        }
      }
      return best?.ir_node_id ?? null;
    }
    if (runningExec) return runningExec.ir_node_id;
    const status = snapshot?.run?.status;
    if (status === "running" || status === "paused_waiting_human") {
      return lastRunningNodeId;
    }
    return null;
  }, [runningExec, scrubSeq, events, snapshot?.run?.status, lastRunningNodeId]);

  const wfSelectedNodeId =
    followLiveNode && followLiveNodeId ? followLiveNodeId : manualSelectedNodeId;

  useRunKeyboard({
    selectedNodeId: wfSelectedNodeId,
    executions: liveExecutions,
    iterationByNode,
    onSelectNode: handleSelectNode,
    onSelectIteration: handleSelectIteration,
    onScrubLive: () => setScrubSeq(null),
    onJumpToFailed: firstFailedNodeId
      ? () => handleSelectNode(firstFailedNodeId)
      : undefined,
  });

  // Everything the canvas / detail / event log render from the current
  // scrub position (live data when scrubSeq is null) — see
  // useDisplayedRunData. deferredScrubSeq stays internal to the hook.
  const { displayedExecutions, displayedEvents, logClampBytes, runtimeOverrideByNode } =
    useDisplayedRunData(scrubSeq, events, liveExecutions);

  // Workflow view: detail panel reflects the selected node's currently-
  // picked iteration. We expose the full per-node execution list plus
  // the resolved iteration so the panel can render an in-place pill
  // strip and switch which exec drives the tabs without round-tripping
  // through the parent. Sort by `first_seq` (start order) — `loop_iteration`
  // is no longer monotonic post-Option-3 (outer-loop counters can
  // dominate inner ones for many iterations), so sorting on it would
  // scramble the pill order vs. canvas pill order.
  //
  // Note: `selectedIteration` here (and in iterationByNode) is a
  // 0-based array INDEX, not the scalar `loop_iteration` field on
  // ExecutionState. See RunCanvasIR.defaultIterationFor for the
  // motivation (nested loops produce multiple execs with the same
  // `loop_iteration`).
  const selectedNodeExecutions = useMemo(() => {
    if (!wfSelectedNodeId) return [] as ExecutionState[];
    return displayedExecutions
      .filter((e) => e.ir_node_id === wfSelectedNodeId)
      .slice()
      .sort((a, b) => a.first_seq - b.first_seq);
  }, [wfSelectedNodeId, displayedExecutions]);

  const selectedNodeIteration = useMemo(() => {
    if (!wfSelectedNodeId || selectedNodeExecutions.length === 0) return 0;
    return (
      iterationByNode.get(wfSelectedNodeId) ??
      defaultIterationFor(selectedNodeExecutions)
    );
  }, [wfSelectedNodeId, iterationByNode, selectedNodeExecutions]);

  // Kept as a local resolved value for the EventLog filter + queued
  // banner predicate below. Mirrors the resolution the detail panel
  // does internally. Clamps so the panel stays useful when the index
  // points past the current array length (e.g. transient race
  // during a fan-in).
  const detailExec = useMemo(() => {
    if (selectedNodeExecutions.length === 0) return null;
    const i = Math.min(
      Math.max(selectedNodeIteration, 0),
      selectedNodeExecutions.length - 1,
    );
    return selectedNodeExecutions[i] ?? null;
  }, [selectedNodeExecutions, selectedNodeIteration]);

  if (!runId) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center gap-3 p-8 text-center">
        <h2 className="text-base font-semibold text-fg-default">No run selected</h2>
        <p className="text-xs text-fg-muted max-w-sm">
          Pick a run from the list, or launch a workflow from the editor to create
          one.
        </p>
      </div>
    );
  }
  if (!snapshot) {
    if (loadFailed) {
      return <RunViewLoadError runId={runId} status={loadFailed.status} message={loadFailed.message} />;
    }
    return <RunViewSkeleton />;
  }

  const active = snapshot.run.status === "running";
  // Drive the EventLog filter from the canvas/detail selection so a
  // click on a node implicitly narrows the event stream.
  const eventLogSelection = detailExec?.execution_id ?? null;
  const liveSeq = snapshot.last_seq;
  const scrubbing = scrubSeq !== null;
  // The browser pane mounts on the right column only when the user
  // chose that dock AND there's something to display. Otherwise it
  // either stays in the bottom tab list, or stays hidden.
  const browserRightDocked = browserDock === "right" && browserAvailable;

  // Pre-pickup state: the run sits on the NATS queue. RunMetrics and
  // Scrubber render nothing useful (no events yet, no budget consumed),
  // so we swap them for the QueuedBanner that surfaces position and
  // exposes a cancel button. The IR canvas stays mounted underneath so
  // the workflow shape is still visible. See cloud-ready plan §F (T-15).
  const isQueued = snapshot.run.status === "queued";
  const isTerminal =
    snapshot.run.status === "finished" ||
    snapshot.run.status === "failed" ||
    snapshot.run.status === "cancelled";
  // The chat input is hidden when the run reached a terminal status,
  // but the transcript stays readable in the floating / docked panel.
  const chatInputDisabled = isQueued || isTerminal;
  const chatDockedRight = chatDock === "docked-right";

  // Pick the layout persistence handle once, indexed by the active
  // column set so we don't repeat the same 4-way ternary at every
  // {defaultLayout, onLayoutChanged, defaultSize}. Stays in render
  // — the four sources don't share an identity and useMemo would
  // capture stale onChange callbacks.
  const horizPersistence = browserRightDocked && chatDockedRight
    ? horizontalLayoutWithBrowserAndChat
    : browserRightDocked
    ? horizontalLayoutWithBrowser
    : chatDockedRight
    ? horizontalLayoutWithChat
    : horizontalLayout;
  const canvasSize = browserRightDocked && chatDockedRight
    ? 40
    : browserRightDocked || chatDockedRight
    ? 50
    : 70;
  const detailSize = browserRightDocked || chatDockedRight ? 22 : 30;
  const browserRightSize = chatDockedRight ? 18 : 25;
  const chatPanelSize = browserRightDocked ? 20 : 30;

  return (
    <ReactFlowProvider>
      <div className="h-full w-full overflow-hidden flex flex-col">
        <RunHeader run={snapshot.run} active={active} wsState={wsState} />
        {isQueued ? (
          <QueuedBanner run={snapshot.run} />
        ) : (
          <>
            <div className="border-b border-border-default bg-surface-1 flex items-stretch">
              <div className="flex-shrink-0">
                <RunMetrics
                  active={active}
                  onJumpToFailed={handleJumpToFailed}
                  bare
                />
              </div>
              {liveSeq > 0 && (
                <div className="flex-1 min-w-0 border-l border-border-default">
                  <Scrubber
                    events={events}
                    liveSeq={liveSeq}
                    scrubSeq={scrubSeq}
                    onChange={setScrubSeq}
                    visible
                    bare
                  />
                </div>
              )}
            </div>
            {snapshot.run.status === "paused_operator" && (
              <OperatorPauseBanner run={snapshot.run} />
            )}
          </>
        )}
      <div className="flex-1 min-h-0 flex">
        <LeftPanel
          runId={runId}
          run={snapshot.run}
          onSelectFile={handleSelectFile}
          onEditFile={handleEditFile}
          onMergeComplete={refreshSnapshot}
        />
        <div className="flex-1 min-h-0 flex flex-col">
          <Group
            orientation="vertical"
            className="flex-1 min-h-0"
            defaultLayout={verticalLayout.layout}
            onLayoutChanged={verticalLayout.onChange}
          >
            <Panel id="top" defaultSize={70} minSize={30} className="min-h-0">
              <Group
                orientation="horizontal"
                className="h-full w-full"
                // Key on the active column set so react-resizable-panels
                // redistributes flexGrow cleanly on toggle instead of
                // carrying over the previous mode's sizing.
                key={`h-${browserRightDocked ? "b" : "_"}-${chatDockedRight ? "c" : "_"}`}
                defaultLayout={horizPersistence.layout}
                onLayoutChanged={horizPersistence.onChange}
              >
                <Panel
                  id="canvas"
                  defaultSize={canvasSize}
                  minSize={25}
                  className="min-h-0"
                >
                  <div className={scrubbing ? "h-full w-full saturate-50" : "h-full w-full"}>
                    <RunCanvasIR
                      runId={runId}
                      executions={displayedExecutions}
                      selectedNodeId={wfSelectedNodeId}
                      onSelectNode={handleSelectNode}
                      iterationByNode={iterationByNode}
                      onSelectIteration={handleSelectIteration}
                      runtimeOverrideByNode={runtimeOverrideByNode}
                      followLive={followLiveNode}
                      onToggleFollowLive={handleToggleFollowLive}
                    />
                  </div>
                </Panel>
                {!detailCollapsed && (
                  <>
                    <ResizeSeparator orientation="horizontal" />
                    <Panel
                      id="detail"
                      defaultSize={detailSize}
                      minSize={18}
                      className="min-h-0"
                    >
                      <div className="h-full border-l border-border-default min-h-0 overflow-hidden animate-fade-in-opacity">
                        <NodeDetailPanel
                          runId={runId}
                          filePath={snapshot.run.file_path}
                          executions={selectedNodeExecutions}
                          selectedIteration={selectedNodeIteration}
                          onSelectIteration={handleSelectIteration}
                          events={displayedEvents}
                          followLive={followLiveNode}
                          onToggleFollowLive={handleToggleFollowLive}
                          subscribeLogs={wsHandle.subscribeLogs}
                          unsubscribeLogs={wsHandle.unsubscribeLogs}
                          onCollapse={toggleDetailCollapsed}
                          logClampBytes={logClampBytes}
                        />
                      </div>
                    </Panel>
                  </>
                )}
                {browserRightDocked && (
                  <>
                    <ResizeSeparator orientation="horizontal" />
                    <Panel
                      id="browserRight"
                      defaultSize={browserRightSize}
                      minSize={18}
                      className="min-h-0"
                    >
                      <div className="h-full border-l border-border-default min-h-0 overflow-hidden animate-fade-in-opacity">
                        <BrowserPane
                          runId={runId}
                          scrubSeq={scrubSeq}
                          dock={browserDock}
                          onDockChange={setBrowserDock}
                        />
                      </div>
                    </Panel>
                  </>
                )}
                {chatDockedRight && (
                  <>
                    <ResizeSeparator orientation="horizontal" />
                    <Panel
                      id="chat"
                      defaultSize={chatPanelSize}
                      minSize={20}
                      className="min-h-0"
                    >
                      <ChatPanelContent
                        runId={runId}
                        inputDisabled={chatInputDisabled}
                        onUndock={() => setChatDock("floating")}
                        onClose={() => setChatDock("closed")}
                      />
                    </Panel>
                  </>
                )}
              </Group>
            </Panel>
            {!eventlogCollapsed && (
              <>
                <ResizeSeparator orientation="vertical" />
                <Panel
                  id="eventlog"
                  defaultSize={30}
                  minSize={10}
                  className="min-h-0"
                >
                  <div className="h-full border-t border-border-default min-h-0 overflow-hidden animate-fade-in-opacity flex flex-col bg-surface-1">
                    <Tabs
                      value={bottomTab}
                      onValueChange={(v) => handleSetBottomTab(v as BottomTab)}
                      items={BOTTOM_TABS.filter(
                        (t) => t !== "browser" || (browserAvailable && !browserRightDocked),
                      ).map((t) => ({ value: t, label: BOTTOM_TAB_LABELS[t] }))}
                      variant="underline"
                      listClassName="px-3"
                    />
                    <div className="flex-1 min-h-0">
                      {bottomTab === "events" ? (
                        <EventLog
                          events={displayedEvents}
                          selectedExecutionId={eventLogSelection}
                          followTail={followTail && !scrubbing}
                          onToggleFollow={setFollowTail}
                          onSelectNodeIteration={handleEventSelect}
                          onClearSelection={handleClearSelection}
                          onCollapse={toggleEventlogCollapsed}
                          runId={runId}
                        />
                      ) : bottomTab === "logs" ? (
                        <RunLogPanel
                          runId={runId}
                          subscribeLogs={wsHandle.subscribeLogs}
                          unsubscribeLogs={wsHandle.unsubscribeLogs}
                          onCollapse={toggleEventlogCollapsed}
                          clampToBytes={logClampBytes}
                        />
                      ) : bottomTab === "browser" && runId ? (
                        <BrowserPane
                          runId={runId}
                          scrubSeq={scrubSeq}
                          dock={browserDock}
                          onDockChange={setBrowserDock}
                        />
                      ) : bottomTab === "artifacts" ? (
                        <ArtifactFilesPanel runId={runId} />
                      ) : (
                        <ReportTab onSelectNode={handleSelectNode} />
                      )}
                    </div>
                  </div>
                </Panel>
              </>
            )}
          </Group>
          {eventlogCollapsed && (
            <ExpandStrip
              orientation="bottom"
              label="Show event log"
              onClick={toggleEventlogCollapsed}
            />
          )}
        </div>
        {detailCollapsed && (
          <ExpandStrip
            orientation="right"
            label="Show details panel"
            onClick={toggleDetailCollapsed}
          />
        )}
      </div>
        <FileDiffDialog
          runId={runId}
          file={diffFile}
          mode={diffMode}
          onClose={() => setDiffFile(null)}
          onEdit={handleEditFile}
        />
        <FileEditDialog
          runId={runId}
          path={editFile}
          onClose={() => setEditFile(null)}
        />
        <FloatingChatPanel
          runId={runId}
          dock={chatDock}
          onDockChange={setChatDock}
          inputDisabled={chatInputDisabled}
        />
      </div>
    </ReactFlowProvider>
  );
}

