import {
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useLocation } from "wouter";
import { ChevronRightIcon } from "@radix-ui/react-icons";

import type { ArtifactSummary, ExecutionState, RunEvent } from "@/api/runs";
import { listArtifacts } from "@/api/runs";
import { CopyButton, IconButton, Input, LiveDot, Popover, StatusBadge, Tabs } from "@/components/ui";
import { stepIteration } from "@/lib/eventIter";
import { formatContextUsage, formatDurationBetween, formatMs } from "@/lib/format";
import { readBooleanFlag, writeBooleanFlag } from "@/lib/localStorageFlag";
import { readNodeOutputMeta } from "@/lib/delegateMeta";
import { softColor } from "@/lib/constants";
import { useToggleSet } from "@/hooks/useToggleSet";
import { NodeIcon } from "@/components/icons/NodeIcon";
import type { NodeKind } from "@/api/types";

import ArtifactDiff from "./ArtifactDiff";
import { iterationColor } from "./IRNode";
import LogLinesView from "./LogLinesView";
import PauseForm from "./PauseForm";
import { statusClasses } from "./runStatusClasses";
import { useLLMSteps, LLMTraceView } from "./detail/LLMTrace";
import { useToolCalls, ToolCallList } from "./detail/ToolCalls";

interface Props {
  runId: string;
  // The workflow source path for this run; used to wire "Open in editor".
  filePath?: string;
  // All executions of the selected IR node, ordered by start time.
  // Empty array = no node selected. The active `exec` is derived from
  // this list + `selectedIteration` (a 0-based array index) inside the
  // panel so the iteration pills can switch which exec drives every
  // tab without round-trip through the parent.
  executions: ExecutionState[];
  // 0-based index into `executions` of the currently selected attempt.
  selectedIteration: number;
  onSelectIteration: (nodeId: string, index: number) => void;
  events: RunEvent[];
  // followLive == true → the parent is auto-tracking the running
  // execution; clicking the toggle off pins the panel on the current
  // exec. Clicking it on again re-engages auto-tracking, which the
  // parent implements by clearing the manual pin in handleToggle.
  followLive?: boolean;
  onToggleFollowLive?: () => void;
  // Imperative log subscription handles forwarded from RunView. The
  // per-node Logs tab subscribes while it's mounted; the hook
  // ref-counts so this coexists with the bottom RunLogPanel.
  subscribeLogs: (fromOffset?: number) => void;
  unsubscribeLogs: () => void;
  onCollapse?: () => void;
  // Byte offset to clamp the per-node Logs tab to during scrub /
  // replay. Forwarded from RunView's events[scrubSeq].log_offset so
  // the panel mirrors the bottom-log panel's rewind.
  logClampBytes?: number | null;
}

function CollapseButton({ onCollapse }: { onCollapse?: () => void }): ReactNode {
  if (!onCollapse) return null;
  return (
    <IconButton
      label="Hide details panel"
      size="sm"
      variant="ghost"
      className="absolute top-1.5 right-1.5 z-10"
      onClick={onCollapse}
    >
      <ChevronRightIcon />
    </IconButton>
  );
}

// FollowLivePill toggles the parent's auto-tracking of the running
// node. When active, the panel jumps to whatever the engine is
// currently working on; when off, the user's manual selection stays
// pinned. The visual is a pill with a pulsing dot when active so it
// reads as "live" at a glance.
function FollowLivePill({
  followLive,
  onToggle,
}: {
  followLive: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] border transition-colors ${
        followLive
          ? "bg-success-soft border-success text-success-fg"
          : "bg-surface-1 border-border-default text-fg-subtle hover:text-fg-default"
      }`}
      title={
        followLive
          ? "Auto-following the running node. Click to pin on the current selection."
          : "Pinned. Click to follow the running node."
      }
    >
      <LiveDot tone={followLive ? "success" : "neutral"} size="sm" pulse={followLive} />
      live
    </button>
  );
}

// IterationPills mirrors the per-iteration pip strip already shown on
// the canvas node (IRNode), but in the right-panel header. The colors
// come from the same ITERATION_PALETTE so a user scanning the canvas
// for "iter 3" sees the same amber tint in both places. Status is
// overlaid as a ring/animation so the pill carries two dimensions:
// which iteration (color) and how it went (ring/pulse/opacity).
function IterationPills({
  executions,
  selectedIteration,
  onSelect,
}: {
  executions: ExecutionState[];
  // 0-based index into `executions` of the currently selected attempt.
  selectedIteration: number;
  // Callback receives the selected attempt's array index.
  onSelect: (index: number) => void;
}) {
  return (
    <div className="mt-1 flex flex-wrap items-center gap-1">
      <span className="text-[9px] text-fg-subtle mr-0.5">iter:</span>
      {executions.map((e, idx) => {
        const isSelected = idx === selectedIteration;
        const s = statusClasses(e.status);
        const color = iterationColor(idx);
        // Selection is rendered as a thicker ring in accent color so
        // the active pill pops; the iteration color stays as the
        // fill. Status drives extra cues:
        //   running → animate-pulse (matches StatusBadge running)
        //   failed  → red ring overlay
        //   skipped → desaturated/opacity (engine bypassed this iter)
        const pulse = e.status === "running" ? "animate-pulse" : "";
        const opacity = e.status === "skipped" ? "opacity-50" : "";
        const failedRing = e.status === "failed" ? "ring-1 ring-danger" : "";
        const selectedRing = isSelected
          ? "ring-2 ring-accent shadow-sm"
          : "ring-1 ring-border-default/30";
        return (
          <button
            key={e.execution_id}
            type="button"
            onClick={() => onSelect(idx)}
            title={`iter ${idx + 1} · ${s.label}`}
            className={`inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full text-[9px] font-mono text-fg-default transition-all ${pulse} ${opacity} ${selectedRing} ${failedRing}`}
            style={{ backgroundColor: softColor(color, 40) }}
          >
            {idx + 1}
          </button>
        );
      })}
    </div>
  );
}

type TabValue = "pause" | "trace" | "tools" | "artifact" | "events" | "logs";

export default function NodeDetailPanel({
  runId,
  filePath,
  executions,
  selectedIteration,
  onSelectIteration,
  events,
  followLive,
  onToggleFollowLive,
  subscribeLogs,
  unsubscribeLogs,
  onCollapse,
  logClampBytes = null,
}: Props) {
  const [artifactVersions, setArtifactVersions] = useState<ArtifactSummary[]>([]);
  const [activeTab, setActiveTab] = useState<TabValue | null>(null);

  // The active execution drives every tab. selectedIteration is the
  // 0-based index in `executions` (NOT a scalar loop_iteration — see
  // RunCanvasIR.defaultIterationFor comment). Clamp on out-of-range
  // so the panel stays useful when the selection points past the
  // current array length (e.g. transient race during a fan-in).
  const exec = useMemo<ExecutionState | null>(() => {
    if (executions.length === 0) return null;
    const i = Math.min(Math.max(selectedIteration, 0), executions.length - 1);
    return executions[i] ?? null;
  }, [executions, selectedIteration]);

  // Load only the version index here; ArtifactDiff handles fetching the
  // body for each selected version on demand.
  //
  // Depending on `exec` as a whole would refire on every WS event
  // (the parent rebuilds the executions array on each event, which
  // re-derives `exec` to a fresh object identity even for the same
  // logical iteration). Pin the effect to the stable scalars only —
  // a node change OR an iteration switch — and abort the in-flight
  // request when those keys change so we never thrash the network
  // pool with redundant /artifacts fetches.
  const execNodeId = exec?.ir_node_id ?? null;
  const executionId = exec?.execution_id ?? null;
  useEffect(() => {
    setArtifactVersions([]);
    if (!execNodeId) return;
    const ctrl = new AbortController();
    listArtifacts(runId, execNodeId, { signal: ctrl.signal })
      .then((summaries) => {
        if (ctrl.signal.aborted) return;
        setArtifactVersions(summaries);
      })
      .catch(() => {
        // Artifacts are best-effort — silent fall-through.
      });
    return () => {
      ctrl.abort();
    };
  }, [runId, execNodeId, executionId]);

  const matching = useExecutionEvents(events, exec);
  const llmSteps = useLLMSteps(matching);
  const toolCalls = useToolCalls(matching);
  const pause = usePauseInfo(matching);

  // Tab default depends on what's most useful for the node kind.
  // We only reset on *node* change (not iteration change within the
  // same node) so flipping between iterations keeps whichever tab the
  // user was reading. Each iteration's tab content rerenders against
  // the new exec via the `matching` events feed.
  //
  // Exception: a paused exec auto-focuses the Pause tab — that one is
  // load-bearing for the human-input flow, so we promote it whenever
  // the active iteration's status is paused (even mid-node).
  const nodeId = exec?.ir_node_id ?? null;
  const isPausedExec = exec?.status === "paused_waiting_human";
  // Derive kind as a stable scalar so we can add it to the effect
  // dep list without resetting on every exec reference change. Without
  // this, an exec that arrives in the same React batch as a nodeId
  // change has the stale closure see exec === null and set
  // activeTab = null, blanking the panel.
  const execKind = exec?.kind ?? null;
  useEffect(() => {
    if (!execKind) {
      setActiveTab(null);
      return;
    }
    if (isPausedExec) {
      setActiveTab("pause");
      return;
    }
    if (execKind === "agent" || execKind === "judge") setActiveTab("trace");
    else if (execKind === "tool") setActiveTab("tools");
    else setActiveTab("events");
    // Iteration flips reuse the user's tab pick — that's why we don't
    // dep on the full `exec` object. nodeId + execKind + isPausedExec
    // cover the cases that warrant a default-tab reset.
  }, [nodeId, execKind, isPausedExec]);

  if (!exec) {
    return (
      <div className="relative h-full p-4 text-xs text-fg-subtle">
        <CollapseButton onCollapse={onCollapse} />
        {onToggleFollowLive && (
          <FollowLivePill
            followLive={!!followLive}
            onToggle={onToggleFollowLive}
          />
        )}
        {followLive ? (
          <p className="mt-8">
            Following the running node. Nothing is executing right now —
            this panel will jump in as soon as the engine starts a node.
          </p>
        ) : (
          <p className="mt-8">
            Click a node to see its events, prompt, response, artifact,
            and error trace.
          </p>
        )}
      </div>
    );
  }

  const hasArtifact = artifactVersions.length > 0;
  const isPaused = exec.status === "paused_waiting_human";

  // Tab order: Pause first when paused (drives action), then Trace,
  // Tools, Artifact, Events. Pause hidden when not paused.
  const tabItems: Array<{ value: TabValue; label: string; disabled?: boolean }> = [
    ...(isPaused
      ? [{ value: "pause" as TabValue, label: "Pause" }]
      : []),
    {
      value: "trace" as TabValue,
      label: llmSteps.length > 1 ? `Trace (${llmSteps.length})` : "Trace",
      disabled: llmSteps.length === 0,
    },
    {
      value: "tools" as TabValue,
      label: `Tools (${toolCalls.length})`,
      disabled: toolCalls.length === 0,
    },
    {
      value: "artifact" as TabValue,
      label:
        artifactVersions.length > 1
          ? `Artifact (${artifactVersions.length})`
          : "Artifact",
      disabled: !hasArtifact,
    },
    { value: "events" as TabValue, label: `Events (${matching.length})` },
    { value: "logs" as TabValue, label: "Logs" },
  ];

  return (
    <div className="relative h-full flex flex-col text-xs">
      <CollapseButton onCollapse={onCollapse} />
      <DetailHeader
        runId={runId}
        filePath={filePath}
        exec={exec}
        executions={executions}
        selectedIteration={selectedIteration}
        onSelectIteration={onSelectIteration}
        events={matching}
        followLive={followLive}
        onToggleFollowLive={onToggleFollowLive}
      />

      <Tabs
        value={activeTab ?? "events"}
        onValueChange={(v) => setActiveTab(v as TabValue)}
        items={tabItems}
        variant="underline"
        listClassName="px-3"
        className="flex-1 min-h-0"
        panels={{
          pause: (
            <div className="overflow-auto px-4 py-3 h-full">
              <PauseForm
                runId={runId}
                questions={pause?.questions ?? {}}
                message={pause?.message}
              />
            </div>
          ),
          trace: (
            <div className="overflow-auto px-4 py-3 h-full">
              {llmSteps.length === 0 ? (
                <p className="text-fg-subtle">
                  No LLM activity recorded for this execution.
                </p>
              ) : (
                <LLMTraceView steps={llmSteps} />
              )}
            </div>
          ),
          tools: (
            <div className="overflow-auto px-4 py-3 h-full">
              {toolCalls.length === 0 ? (
                <div className="text-fg-subtle">
                  No tool calls for this execution.
                </div>
              ) : (
                <ToolCallList calls={toolCalls} runId={runId} />
              )}
            </div>
          ),
          artifact: (
            <div className="overflow-auto px-4 py-3 h-full">
              {!hasArtifact ? (
                <div className="text-fg-subtle">No artifact published.</div>
              ) : (
                <ArtifactDiff
                  runId={runId}
                  nodeId={exec.ir_node_id}
                  versions={artifactVersions}
                />
              )}
            </div>
          ),
          events: (
            <div className="overflow-hidden h-full">
              <EventsTabContent events={matching} />
            </div>
          ),
          logs: (
            <LogLinesView
              runId={runId}
              subscribeLogs={subscribeLogs}
              unsubscribeLogs={unsubscribeLogs}
              filterNodeId={exec.ir_node_id}
              filterIteration={exec.loop_iteration}
              showTitle={false}
              clampToBytes={logClampBytes}
            />
          ),
        }}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

// IterationCrumb renders the "iter: N" position in the breadcrumb
// row. When the node has only one execution it stays a static label;
// for multi-iteration nodes it becomes a button that opens a popover
// listing every attempt with status + duration so the user can jump
// between iterations without leaving the right pane.
function IterationCrumb({
  exec,
  executions,
  selectedIteration,
  onSelect,
}: {
  exec: ExecutionState;
  executions: ExecutionState[];
  selectedIteration: number;
  onSelect: (iter: number) => void;
}): ReactNode {
  const idx = executions.findIndex((e) => e.execution_id === exec.execution_id);
  const position = idx >= 0 ? idx + 1 : 1;
  const total = executions.length;
  // Declared before the single-execution early return so the hook is
  // unconditional (rules-of-hooks); unused when total <= 1.
  const [open, setOpen] = useState(false);
  if (total <= 1) {
    return <span>iter: {position}</span>;
  }
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      side="bottom"
      align="start"
      contentClassName="min-w-[220px] p-1.5 text-[11px]"
      trigger={
        <button
          type="button"
          className="hover:text-fg-default underline-offset-2 hover:underline"
          title="Jump to a different iteration"
        >
          iter: {position}/{total}
        </button>
      }
    >
      <ul className="space-y-0.5">
        {executions.map((e, i) => {
          const active = i === selectedIteration;
          const duration = formatDurationBetween(e.started_at, e.finished_at);
          return (
            <li key={e.execution_id}>
              <button
                type="button"
                onClick={() => {
                  onSelect(i);
                  setOpen(false);
                }}
                className={`w-full text-left px-2 py-1 rounded flex items-center gap-2 ${
                  active
                    ? "bg-accent-soft text-fg-default"
                    : "hover:bg-surface-2 text-fg-muted"
                }`}
              >
                <span className="font-mono w-6 shrink-0">#{i + 1}</span>
                <StatusBadge status={e.status} />
                {duration && (
                  <span className="text-fg-subtle ml-auto">{duration}</span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
    </Popover>
  );
}

function NodeKindIcon({ kind }: { kind?: string }): ReactNode {
  if (!kind) return null;
  return <NodeIcon kind={kind as NodeKind} size={14} />;
}

function DetailHeader({
  runId,
  filePath,
  exec,
  executions,
  selectedIteration,
  onSelectIteration,
  events,
  followLive,
  onToggleFollowLive,
}: {
  runId: string;
  filePath?: string;
  exec: ExecutionState;
  executions: ExecutionState[];
  // 0-based index into `executions` of the currently selected attempt.
  selectedIteration: number;
  onSelectIteration: (nodeId: string, index: number) => void;
  events: RunEvent[];
  followLive?: boolean;
  onToggleFollowLive?: () => void;
}) {
  const [, setLocation] = useLocation();
  const duration = formatDurationBetween(exec.started_at, exec.finished_at);
  // Per-execution cost + tokens are sourced from the node_finished
  // event the runtime emits with the cost.Annotate output. A single
  // execution emits at most one node_finished, so summing across the
  // matching events is just to defensively merge if the engine ever
  // emits multiple (e.g. on retry within a node) — the common case is
  // exactly one row.
  const {
    costUsd,
    tokens,
    model,
    contextWindow,
    contextUsed,
    thinkingTokens,
    thinkingMs,
  } = useMemo(() => {
    let costUsd = 0;
    let tokens = 0;
    let model = "";
    let contextWindow = 0;
    let contextUsed = 0;
    let thinkingTokens = 0;
    let thinkingMs = 0;
    for (const e of events) {
      if (e.type !== "node_finished" || !e.data) continue;
      const c = e.data["_cost_usd"];
      if (typeof c === "number") costUsd += c;
      const t = e.data["_tokens"];
      if (typeof t === "number") tokens += t;
      const meta = readNodeOutputMeta(
        e.data["output"] as Record<string, unknown> | undefined,
      );
      if (meta.model && !model) model = meta.model;
      if (meta.contextWindow && meta.contextWindow > contextWindow)
        contextWindow = meta.contextWindow;
      if (meta.contextUsed && meta.contextUsed > contextUsed)
        contextUsed = meta.contextUsed;
      if (meta.thinkingTokens) thinkingTokens += meta.thinkingTokens;
      if (meta.thinkingMs) thinkingMs += meta.thinkingMs;
    }
    return {
      costUsd,
      tokens,
      model,
      contextWindow,
      contextUsed,
      thinkingTokens,
      thinkingMs,
    };
  }, [events]);
  const contextUsage = formatContextUsage(contextUsed, contextWindow);
  // Wall-clock vs relative durations. Persisted so the preference
  // survives reloads — operators correlating runs with CI/dashboards
  // need wall-clock anchors and shouldn't have to re-toggle each visit.
  const [showAbsoluteTimes, setShowAbsoluteTimes] = useState<boolean>(() =>
    readBooleanFlag("run-console.absolute-times"),
  );
  useEffect(() => {
    writeBooleanFlag("run-console.absolute-times", showAbsoluteTimes);
  }, [showAbsoluteTimes]);

  return (
    <div className="px-4 pt-3 pb-3 pr-10 border-b border-border-default">
      <div className="flex items-start gap-2">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <StatusBadge status={exec.status} />
            <NodeKindIcon kind={exec.kind} />
            <h2 className="text-sm font-semibold truncate" title={exec.ir_node_id}>
              {exec.ir_node_id}
            </h2>
            {onToggleFollowLive && (
              <FollowLivePill
                followLive={!!followLive}
                onToggle={onToggleFollowLive}
              />
            )}
          </div>
          <div className="text-fg-subtle text-[10px] flex flex-wrap gap-x-3 gap-y-0.5">
            {exec.kind && <span>kind: {exec.kind}</span>}
            <span>branch: {exec.branch_id}</span>
            <IterationCrumb
              exec={exec}
              executions={executions}
              selectedIteration={selectedIteration}
              onSelect={(iter) => onSelectIteration(exec.ir_node_id, iter)}
            />
            {duration && (
              <button
                type="button"
                onClick={() => setShowAbsoluteTimes((v) => !v)}
                className="hover:text-fg-default text-left"
                title={
                  showAbsoluteTimes
                    ? "Click for relative duration"
                    : "Click for absolute wall-clock anchors"
                }
              >
                {showAbsoluteTimes && exec.started_at
                  ? `${formatWallClock(exec.started_at)} → ${
                      exec.finished_at ? formatWallClock(exec.finished_at) : "…"
                    }`
                  : `duration: ${duration}`}
              </button>
            )}
            {tokens > 0 && <span>tokens: {tokens.toLocaleString()}</span>}
            {(thinkingTokens > 0 || thinkingMs > 0) && (
              <span
                title="Extended thinking. Token count is an approximation (the provider bills thinking inside output tokens; the text is re-encoded). Time is measured (exact for claw, best-effort for claude_code)."
              >
                🧠 ~{thinkingTokens.toLocaleString()} tok · {formatMs(thinkingMs)}
              </span>
            )}
            {costUsd > 0 && (
              <span title={`$${costUsd.toFixed(6)}`}>
                cost: ${costUsd.toFixed(4)}
              </span>
            )}
            {model && <span className="font-mono">{model}</span>}
            {contextUsage && (
              <span title={contextUsage.title}>
                ctx: {contextUsage.label} ({Math.round(contextUsage.pct)}%)
              </span>
            )}
          </div>
          {executions.length > 1 && (
            <IterationPills
              executions={executions}
              selectedIteration={selectedIteration}
              onSelect={(iter) => onSelectIteration(exec.ir_node_id, iter)}
            />
          )}
        </div>
        {filePath && (
          <IconButton
            label="Open in editor"
            tooltip="Open this node in the studio"
            size="sm"
            variant="ghost"
            onClick={() =>
              setLocation(
                `/editor?file=${encodeURIComponent(filePath)}&node=${encodeURIComponent(
                  exec.ir_node_id,
                )}&from=${encodeURIComponent(runId)}`,
              )
            }
          >
            ↗
          </IconButton>
        )}
      </div>

      {exec.error && (
        <div className="mt-2 px-2 py-1.5 rounded bg-danger-soft text-danger-fg">
          <div className="flex items-center justify-between gap-2 mb-0.5">
            <span className="font-medium">Error</span>
            <CopyButton value={exec.error} />
          </div>
          <div className="font-mono text-[11px] whitespace-pre-wrap break-words">
            {exec.error}
          </div>
        </div>
      )}
    </div>
  );
}


// ---------------------------------------------------------------------------
// Pause tab
// ---------------------------------------------------------------------------

interface PauseInfo {
  questions: Record<string, unknown>;
  message?: string;
}

function usePauseInfo(matching: RunEvent[]): PauseInfo | null {
  return useMemo<PauseInfo | null>(() => {
    // Walk newest → oldest looking for the most recent
    // human_input_requested for this execution. The reducer in the
    // store flips status back to running on resume, so it's safe to
    // assume the latest pause request is the active one.
    for (let i = matching.length - 1; i >= 0; i--) {
      const e = matching[i]!;
      if (e.type === "human_input_requested" && e.data) {
        return {
          questions:
            (e.data["questions"] as Record<string, unknown> | undefined) ?? {},
          message:
            (e.data["message"] as string | undefined) ??
            (e.data["reason"] as string | undefined),
        };
      }
    }
    return null;
  }, [matching]);
}

// ---------------------------------------------------------------------------
// Events tab
// ---------------------------------------------------------------------------

function EventsTabContent({ events }: { events: RunEvent[] }) {
  const [search, setSearch] = useState("");
  const activeTypesSet = useToggleSet<string>();
  const activeTypes = activeTypesSet.set;
  const [showRawData, setShowRawData] = useState(false);

  // Counts per type so the chip row can show occurrence numbers.
  const typeCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of events) m.set(e.type, (m.get(e.type) ?? 0) + 1);
    return m;
  }, [events]);

  const knownTypes = useMemo(
    () => Array.from(typeCounts.keys()).sort(),
    [typeCounts],
  );

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return events.filter((e) => {
      if (activeTypes.size > 0 && !activeTypes.has(e.type)) return false;
      if (!query) return true;
      // Search type, node_id, and stringified data so users can grep
      // for a substring (e.g. "rate_limit" or "tool_name=Bash").
      if (e.type.toLowerCase().includes(query)) return true;
      if (e.node_id?.toLowerCase().includes(query)) return true;
      if (e.data && JSON.stringify(e.data).toLowerCase().includes(query))
        return true;
      return false;
    });
  }, [events, activeTypes, search]);

  const toggleType = activeTypesSet.toggle;

  return (
    <div className="h-full flex flex-col">
      <div className="px-4 py-2 border-b border-border-default space-y-1.5">
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search events…"
          size="sm"
          leadingIcon={<span className="text-[10px]">⌕</span>}
        />
        {knownTypes.length > 1 && (
          <div className="flex flex-wrap gap-1">
            {knownTypes.map((t) => {
              const isActive = activeTypes.has(t);
              return (
                <button
                  key={t}
                  type="button"
                  onClick={() => toggleType(t)}
                  className={`text-[10px] px-1.5 py-0.5 rounded border transition-colors ${
                    isActive
                      ? "bg-accent-soft border-accent text-fg-default"
                      : "bg-surface-1 border-border-default text-fg-subtle hover:text-fg-default"
                  }`}
                >
                  {t} <span className="text-fg-subtle">{typeCounts.get(t)}</span>
                </button>
              );
            })}
          </div>
        )}
        <div className="flex items-center gap-2">
          <span className="text-[10px] text-fg-subtle">
            {filtered.length} / {events.length} events
          </span>
          <button
            type="button"
            onClick={() => setShowRawData((v) => !v)}
            className="ml-auto text-[10px] text-fg-subtle hover:text-fg-default"
          >
            {showRawData ? "hide raw" : "show raw"}
          </button>
        </div>
      </div>
      <div className="flex-1 overflow-auto px-4 py-2">
        {filtered.length === 0 ? (
          <div className="text-fg-subtle">No events match.</div>
        ) : (
          <ul className="space-y-0.5 font-mono text-[10px]">
            {filtered.map((e) => (
              <li key={`${e.run_id}:${e.seq}`}>
                <div className="flex gap-2">
                  <span className="text-fg-subtle">
                    {e.seq.toString().padStart(4, "0")}
                  </span>
                  <span>{e.type}</span>
                </div>
                {showRawData && e.data && Object.keys(e.data).length > 0 && (
                  <pre className="ml-12 my-0.5 text-fg-subtle whitespace-pre-wrap break-all">
                    {JSON.stringify(e.data, null, 2)}
                  </pre>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function useExecutionEvents(events: RunEvent[], exec: ExecutionState | null) {
  return useMemo<RunEvent[]>(() => {
    if (!exec) return [];
    const out: RunEvent[] = [];
    const counts = new Map<string, number>();
    for (const e of events) {
      if (!e.node_id) continue;
      const iter = stepIteration(counts, e);
      if (
        (e.branch_id || "main") === exec.branch_id &&
        e.node_id === exec.ir_node_id &&
        iter === exec.loop_iteration
      ) {
        out.push(e);
      }
    }
    return out;
  }, [events, exec]);
}

// formatWallClock renders an ISO timestamp as HH:MM:SS in the user's
// locale so the duration cell can flip to absolute wall-clock anchors
// (R16). Falls back to the raw input when parsing fails — better to
// show something than silently lose the timestamp.
function formatWallClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// Re-export needed by EventLog (Phase 6 will own it).
export type { TabValue };
