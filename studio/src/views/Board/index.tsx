import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearch } from "wouter";

import { formatRelative } from "@/lib/format";
import { readBooleanFlag, writeBooleanFlag } from "@/lib/localStorageFlag";

import { useHeaderSlot } from "@/components/shared/useHeaderSlot";
import DispatcherControlBar from "@/components/shared/DispatcherControlBar";
import { Button } from "@/components/ui/Button";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { EmptyState } from "@/components/ui/EmptyState";
import { softColor } from "@/lib/constants";
import {
  cancelIssue,
  getState,
  type DispatchSkipView,
  type RetryView,
  type RunningView,
} from "@/api/dispatcher";
import {
  createIssue,
  deleteIssue,
  getBoard,
  listIssues,
  patchIssue,
  transitionIssue,
  type NativeBoard,
  type NativeIssue,
  type NativeIssuePatch,
} from "@/api/native";
import IssueModal from "./IssueModal";
import SettingsDrawer from "@/components/Dispatcher/SettingsDrawer";
import TrackerErrorBanner from "@/components/shared/TrackerErrorBanner";
import { useBoardKeyboard } from "@/hooks/useBoardKeyboard";
import { useConfirm } from "@/hooks/useConfirm";
import { useUIStore } from "@/store/ui";

// Pre-dispatch lanes: an issue here has not yet entered the dispatcher's
// eligible queue, so it can be "dispatched" (transitioned into the
// dispatch lane). review/done/blocked are downstream and not dispatchable.
function isDispatchable(state: string): boolean {
  return state === "inbox" || state === "backlog";
}

// Above this many at once, bulk dispatch asks for confirmation — each
// dispatch starts a paid run.
const BULK_DISPATCH_CONFIRM_THRESHOLD = 3;

// Priority presets offered by the bulk "Priority" picker (the magnitudes
// the roadmap uses). Columns sort by priority descending by default.
const PRIORITY_PRESETS = [0, 1, 2, 3, 5, 10, 20, 30];

// Max label chips shown on a card before collapsing the rest into "+N".
const MAX_CARD_LABELS = 3;

// Intra-column ordering modes offered by the board's Sort selector.
type SortMode = "priority" | "updated" | "created" | "title";

const SORT_OPTIONS: { value: SortMode; label: string }[] = [
  { value: "priority", label: "Priority" },
  { value: "updated", label: "Recently updated" },
  { value: "created", label: "Recently created" },
  { value: "title", label: "Title (A–Z)" },
];

// sortComparator returns the per-column ordering for a sort mode. Priority
// is descending (higher number first, the board's long-standing default);
// date modes are newest-first; title is alphabetical.
function sortComparator(mode: SortMode): (a: NativeIssue, b: NativeIssue) => number {
  switch (mode) {
    case "updated":
      return (a, b) => (b.updated_at ?? "").localeCompare(a.updated_at ?? "");
    case "created":
      return (a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? "");
    case "title":
      return (a, b) => (a.title ?? "").localeCompare(b.title ?? "");
    case "priority":
    default:
      return (a, b) => (b.priority ?? 0) - (a.priority ?? 0);
  }
}

export default function BoardView() {
  const [, setLocation] = useLocation();
  const search = useSearch();
  const focusFromUrl = useMemo(() => {
    return new URLSearchParams(search).get("focus");
  }, [search]);
  const [board, setBoard] = useState<NativeBoard | null>(null);
  const [issues, setIssues] = useState<NativeIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<NativeIssue | null>(null);
  const [creating, setCreating] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [runningByIssue, setRunningByIssue] = useState<Map<string, RunningView>>(new Map());
  const [retryingByIssue, setRetryingByIssue] = useState<Map<string, RetryView>>(new Map());
  const [skipByIssue, setSkipByIssue] = useState<Map<string, DispatchSkipView>>(new Map());
  const [trackerError, setTrackerError] = useState<{ tracker: string; message: string } | null>(
    null,
  );
  const [dispatcherPaused, setDispatcherPaused] = useState(false);
  // History of recent column transitions, so Ctrl+Z reverts the last
  // one. Bounded at 10 entries — the board's drag-undo intent is the
  // immediate "oops, wrong column", not full session replay.
  const transitionHistoryRef = useRef<Array<{ id: string; from: string }>>([]);
  // Signature of the last-seen active (running + retrying) issue set.
  // The 2s dispatcher poll only refreshes the running/retry overlay; an
  // issue's column is driven by its `state`, which the dispatcher
  // mutates server-side on dispatch (ready→in_progress) and completion
  // (→review/done). When this signature changes we re-fetch issues so a
  // dispatched card actually moves columns instead of stranding in
  // `ready` with a running badge until a manual refresh.
  const prevActiveSigRef = useRef<string>("");
  // Multi-selection state. `selectedIds` is the full set; `anchorId`
  // is the pivot for shift-range extension and the focal point for
  // keyboard navigation. A plain click collapses both to {id}; ctrl/
  // meta-click toggles membership; shift-click extends a range from
  // the anchor to the clicked card without opening the modal.
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());
  const [anchorId, setAnchorId] = useState<string | null>(null);
  const setSingleSelection = useCallback((id: string | null) => {
    if (id === null) {
      setSelectedIds(new Set());
      setAnchorId(null);
    } else {
      setSelectedIds(new Set([id]));
      setAnchorId(id);
    }
  }, []);
  const [helpOpen, setHelpOpen] = useState(false);
  const addToast = useUIStore((s) => s.addToast);
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [searchQuery, setSearchQuery] = useState("");
  const [labelFilter, setLabelFilter] = useState<Set<string>>(() => new Set());
  const [assigneeFilter, setAssigneeFilter] = useState("");
  const [sortMode, setSortMode] = useState<SortMode>("priority");
  // Single source of truth for label filter toggling — used both by
  // the top filter strip and by clicking a chip on any card. Lifted
  // here so card-level chips toggle the same Set the filter strip
  // shows.
  const onLabelToggle = useCallback((l: string) => {
    setLabelFilter((prev) => {
      const next = new Set(prev);
      if (next.has(l)) next.delete(l);
      else next.add(l);
      return next;
    });
  }, []);

  // Poll the dispatcher snapshot every 2s so each card can show a
  // running/retrying badge + cancel button. We ignore failures: when
  // the dispatcher is idle the snapshot is still returned (empty
  // running/retries), and a 5xx is rare enough that flashing the maps
  // empty would be more disruptive than keeping stale data.
  useEffect(() => {
    let alive = true;
    let inflight = false;
    let gen = 0;
    const tick = async () => {
      if (!alive || inflight) return;
      inflight = true;
      const myGen = ++gen;
      try {
        const snap = await getState();
        // Drop responses that arrive after a newer request has
        // started — without the gen guard, a slow getState resolving
        // after a fresh tick would clobber the newer state.
        if (!alive || myGen !== gen) return;
        const rmap = new Map<string, RunningView>();
        for (const r of snap.running ?? []) rmap.set(r.issue_id, r);
        const xmap = new Map<string, RetryView>();
        for (const r of snap.retries ?? []) xmap.set(r.issue_id, r);
        const skmap = new Map<string, DispatchSkipView>();
        for (const s of snap.dispatch_skips ?? []) skmap.set(s.issue_id, s);
        setRunningByIssue(rmap);
        setRetryingByIssue(xmap);
        setSkipByIssue(skmap);
        // When the active (running + retrying) set changes — a dispatch
        // started or a run finished — the affected issue's server-side
        // `state` has moved (ready→in_progress, →review/done), but the
        // poll above only refreshed the overlay. Re-fetch issues so the
        // card actually changes columns. Gated on a set *change* so we
        // don't re-fetch every 2s or fight an in-flight optimistic drag;
        // prevActiveSigRef advances only after a successful fetch so a
        // transient failure retries on the next tick.
        // Tag each id with its map. A running→retry move keeps the same
        // issue id but the dispatcher reverts its server-side state
        // (in_progress→ready); an untagged union still contains that id, so
        // the signature wouldn't change and the card would linger in the
        // in_progress column while the server has it back in ready. Tagging
        // makes "r:id"→"x:id" a signature change, forcing the re-fetch that
        // snaps the card to its real column.
        const activeSig = [
          ...[...rmap.keys()].map((id) => "r:" + id),
          ...[...xmap.keys()].map((id) => "x:" + id),
        ]
          .sort()
          .join(",");
        if (activeSig !== prevActiveSigRef.current) {
          void listIssues()
            .then((fresh) => {
              if (!alive) return;
              setIssues(fresh ?? []);
              prevActiveSigRef.current = activeSig;
            })
            .catch(() => {
              /* leave prevActiveSigRef stale → retry next tick */
            });
        }
        // Guard the tracker error update on value equality so a stable
        // poll doesn't churn an identical object reference each tick
        // and re-render the whole board.
        setTrackerError((prev) => {
          const message = snap.last_tracker_error ?? "";
          if (!message) return prev === null ? prev : null;
          if (
            prev &&
            prev.tracker === snap.tracker &&
            prev.message === message
          ) {
            return prev;
          }
          return { tracker: snap.tracker, message };
        });
        setDispatcherPaused(!!snap.paused);
      } catch {
        // swallow: dispatcher may be unreachable / not wired
      } finally {
        inflight = false;
      }
    };
    void tick();
    const id = setInterval(() => void tick(), 2000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);

  const onCancelRun = useCallback(async (issueID: string) => {
    try {
      await cancelIssue(issueID);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [b, i] = await Promise.all([getBoard(), listIssues()]);
      setBoard(b);
      setIssues(i ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Apply the ?focus=<issueID> deep-link from the Dispatcher view's
  // retry-queue rows. Runs once after issues load so the auto-selected
  // card is actually present in state. Self-clears the param so a hard
  // reload doesn't re-focus on an issue the user has since moved on
  // from.
  useEffect(() => {
    if (!focusFromUrl) return;
    if (issues.length === 0) return;
    const match = issues.find((i) => i.id === focusFromUrl);
    if (!match) return;
    setSingleSelection(match.id);
    setLocation("/board", { replace: true });
  }, [focusFromUrl, issues, setLocation, setSingleSelection]);

  // Distinct values exposed in the filter dropdowns. Derived from the
  // current issues list so the dropdowns track what the user actually
  // sees — including labels created on the fly by bots.
  const { allLabels, allAssignees } = useMemo(() => {
    const labels = new Set<string>();
    const assignees = new Set<string>();
    for (const iss of issues) {
      for (const l of iss.labels ?? []) labels.add(l);
      if (iss.assignee) assignees.add(iss.assignee);
    }
    return {
      allLabels: Array.from(labels).sort(),
      allAssignees: Array.from(assignees).sort(),
    };
  }, [issues]);

  // filteredIssues applies the search query + active label/assignee
  // filters to the raw issues list. Keep filtering client-side: the
  // backend's listIssues filter would force a full network round-trip
  // on every keystroke, which makes the search feel laggy on
  // multi-hundred-issue boards. Title/body substring is case-insensitive.
  const filteredIssues = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    const labels = labelFilter;
    const assignee = assigneeFilter.trim();
    if (!q && labels.size === 0 && !assignee) return issues;
    return issues.filter((iss) => {
      if (q) {
        const hay =
          (iss.title ?? "") + "\t" + (iss.body ?? "") + "\t" + iss.id;
        if (!hay.toLowerCase().includes(q)) return false;
      }
      if (labels.size > 0) {
        const have = new Set(iss.labels ?? []);
        for (const l of labels) {
          if (!have.has(l)) return false;
        }
      }
      if (assignee && iss.assignee !== assignee) return false;
      return true;
    });
  }, [issues, searchQuery, labelFilter, assigneeFilter]);

  // Group filtered issues by state for column rendering. Issues whose
  // state does not appear on the board land in an "unmapped" bucket so
  // they are not silently lost when the operator renames a state.
  const byState = useMemo(() => {
    const m = new Map<string, NativeIssue[]>();
    if (!board) return m;
    for (const s of board.states) m.set(s.name, []);
    m.set("__unmapped__", []);
    for (const iss of filteredIssues) {
      const bucket = m.has(iss.state) ? iss.state : "__unmapped__";
      m.get(bucket)!.push(iss);
    }
    const cmp = sortComparator(sortMode);
    for (const list of m.values()) {
      list.sort(cmp);
    }
    return m;
  }, [board, filteredIssues, sortMode]);

  // Flat issue-id sequence in column-then-row order. Used as the
  // 1-D coordinate space for shift-click range extension across
  // columns: anchor and clicked card are indexed here, everything
  // between them is added to the selection.
  const flatIssueIds = useMemo(() => {
    const flat: string[] = [];
    if (!board) return flat;
    for (const s of board.states) {
      for (const iss of byState.get(s.name) ?? []) flat.push(iss.id);
    }
    for (const iss of byState.get("__unmapped__") ?? []) flat.push(iss.id);
    return flat;
  }, [board, byState]);

  // Eligible counts surfaced in the browser tab title so operators
  // with the board pinned in a background tab see new ready/
  // in-progress work without focusing it. Derived as a stable string
  // first so the effect only runs when the rendered counts actually
  // change — `byState` gets a fresh Map identity every render, which
  // would otherwise rewrite document.title every 2s on every poll
  // tick.
  const tabBadge = useMemo(() => {
    if (!board) return null;
    const eligible = board.states.filter((s) => s.eligible);
    if (eligible.length === 0) return null;
    const parts: string[] = [];
    for (const s of eligible) {
      const count = (byState.get(s.name) ?? []).length;
      if (count > 0) parts.push(`${count} ${s.display ?? s.name}`);
    }
    return parts.length > 0 ? `(${parts.join(", ")})` : null;
  }, [board, byState]);

  useEffect(() => {
    if (!tabBadge) return;
    const prev = document.title;
    document.title = `${tabBadge} ${prev}`;
    return () => {
      document.title = prev;
    };
  }, [tabBadge]);

  const recordTransition = useCallback((id: string, from: string) => {
    const hist = transitionHistoryRef.current;
    hist.push({ id, from });
    if (hist.length > 10) hist.shift();
  }, []);

  const onDrop = useCallback(
    async (issueID: string, toState: string, opts?: { recordHistory?: boolean }) => {
      const recordHistory = opts?.recordHistory ?? true;
      // Capture this invocation's pre-state in a per-call closure so
      // two near-simultaneous drops don't race over the same `before`
      // variable. The prior implementation hoisted `before` to the
      // outer scope and the second drop would overwrite the first
      // drop's snapshot before its async transitionIssue had a chance
      // to fail / roll back, restoring the wrong row.
      const draft: { snapshot: NativeIssue[]; prevState: string | null } = {
        snapshot: [],
        prevState: null,
      };
      setIssues((cur) => {
        draft.snapshot = cur;
        const found = cur.find((i) => i.id === issueID);
        draft.prevState = found?.state ?? null;
        return cur.map((i) => (i.id === issueID ? { ...i, state: toState } : i));
      });
      try {
        await transitionIssue(issueID, toState);
        if (recordHistory && draft.prevState && draft.prevState !== toState) {
          recordTransition(issueID, draft.prevState);
        }
        return true;
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        // Only revert this issue's row to its pre-drop state — leave
        // other concurrent edits in place. Falls back to full revert
        // when the row was reordered out of the snapshot.
        const previous = draft.snapshot;
        setIssues((cur) => {
          const prev = previous.find((i) => i.id === issueID);
          if (!prev) return previous;
          return cur.map((i) => (i.id === issueID ? prev : i));
        });
        // Report failure so batch callers (onBulkDispatch) can
        // distinguish a queued issue from one that never moved — a
        // swallowed transition error here previously let a bulk
        // "Dispatched N" success toast fire even when some issues
        // never reached the dispatch lane.
        return false;
      }
    },
    [recordTransition],
  );

  const undoLastTransition = useCallback(() => {
    const last = transitionHistoryRef.current.pop();
    if (!last) return;
    // Avoid re-recording the undo as a new history entry — otherwise
    // the user would just toggle between two columns forever.
    void onDrop(last.id, last.from, { recordHistory: false });
  }, [onDrop]);

  // Wire Ctrl+Z to the undo-last-transition. Skip when a modal owns
  // focus or an input is active so we don't fight form fields.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.key !== "z" || e.shiftKey) return;
      const target = e.target as HTMLElement | null;
      const inInput =
        target?.tagName === "INPUT" ||
        target?.tagName === "TEXTAREA" ||
        target?.isContentEditable;
      if (inInput) return;
      if (creating || editing || helpOpen || settingsOpen) return;
      if (transitionHistoryRef.current.length === 0) return;
      e.preventDefault();
      undoLastTransition();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [creating, editing, helpOpen, settingsOpen, undoLastTransition]);

  const onCardClick = useCallback(
    (iss: NativeIssue, e: React.MouseEvent) => {
      const meta = e.ctrlKey || e.metaKey;
      const shift = e.shiftKey;

      if (meta) {
        const wasSelected = selectedIds.has(iss.id);
        setSelectedIds((prev) => {
          const next = new Set(prev);
          if (next.has(iss.id)) next.delete(iss.id);
          else next.add(iss.id);
          return next;
        });
        // Anchor follows additions so a follow-up shift-click
        // extends from here. On deselect, keep the prior anchor —
        // unless it was this same card, which is no longer there.
        setAnchorId((prev) =>
          wasSelected ? (prev === iss.id ? null : prev) : iss.id,
        );
        return;
      }

      if (shift && anchorId && anchorId !== iss.id) {
        const startIdx = flatIssueIds.indexOf(anchorId);
        const endIdx = flatIssueIds.indexOf(iss.id);
        if (startIdx >= 0 && endIdx >= 0) {
          const [lo, hi] = startIdx <= endIdx ? [startIdx, endIdx] : [endIdx, startIdx];
          const range = flatIssueIds.slice(lo, hi + 1);
          setSelectedIds((prev) => {
            const next = new Set(prev);
            for (const id of range) next.add(id);
            return next;
          });
        }
        return;
      }

      // Plain click selects only (GitHub-style) — opening the modal is
      // a deliberate gesture (double-click the card or click the title).
      // Selection still updates so the keyboard hook has an anchor and
      // the single-card action bar appears.
      setSingleSelection(iss.id);
    },
    [anchorId, flatIssueIds, selectedIds, setSingleSelection],
  );

  const onCardDragStart = useCallback(
    (iss: NativeIssue, e: React.DragEvent) => {
      // Dragging an unselected card collapses the selection to
      // that card — the operator's intent is "move this one",
      // not "move the four I forgot were still selected".
      let ids: string[];
      if (selectedIds.has(iss.id) && selectedIds.size > 1) {
        ids = Array.from(selectedIds);
      } else {
        ids = [iss.id];
        setSingleSelection(iss.id);
      }
      e.dataTransfer.setData(DRAG_MIME_ISSUE_IDS, JSON.stringify(ids));
      // text/plain fallback so a drag-out into another app yields
      // the originating issue id rather than a JSON blob.
      e.dataTransfer.setData("text/plain", iss.id);
      e.dataTransfer.effectAllowed = "move";
    },
    [selectedIds, setSingleSelection],
  );

  const onColumnDrop = useCallback(
    (ids: string[], toState: string) => {
      for (const id of ids) void onDrop(id, toState);
    },
    [onDrop],
  );

  const onCreate = useCallback(
    async (input: Partial<NativeIssue>) => {
      try {
        await createIssue({
          title: input.title ?? "",
          body: input.body,
          state: input.state,
          labels: input.labels,
          priority: input.priority,
          assignee: input.assignee,
          fields: input.fields,
          bot: input.bot,
          bot_args: input.bot_args,
        });
        setCreating(false);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [refresh],
  );

  const onSave = useCallback(
    async (input: Partial<NativeIssue>) => {
      if (!editing) return;
      try {
        await patchIssue(editing.id, {
          title: input.title,
          body: input.body,
          labels: input.labels,
          priority: input.priority,
          // assignee/bot/bot_args all default to a cleared value ("" / "" /
          // {}) when the operator empties the field, so the corresponding
          // Patch pointer is SET and the server actually clears a
          // previously-stored value. The modal emits `undefined` for an
          // empty field; without the `?? ""` the key is JSON-dropped, the
          // server reads a nil pointer as "unchanged", and the stale value
          // silently persists. For `assignee` that also kept routing the
          // issue to the wrong per-assignee workflow (assignee selects the
          // bot), so clearing it has to reach the store.
          assignee: input.assignee ?? "",
          fields: input.fields,
          bot: input.bot ?? "",
          bot_args: input.bot_args ?? {},
        });
        setEditing(null);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [editing, refresh],
  );

  const onDelete = useCallback(
    async (id: string) => {
      if (
        !(await confirm({
          title: "Delete this issue?",
          message: "This removes it from the board and cannot be undone.",
          confirmLabel: "Delete",
          confirmVariant: "danger",
        }))
      )
        return;
      try {
        await deleteIssue(id);
        setEditing(null);
        setSelectedIds((cur) => {
          if (!cur.has(id)) return cur;
          const next = new Set(cur);
          next.delete(id);
          return next;
        });
        setAnchorId((cur) => (cur === id ? null : cur));
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [confirm, refresh],
  );

  // The dispatch lane: the first eligible, non-terminal state (the
  // "Let's go"/ready column the dispatcher claims from). Falls back to
  // "ready" for boards that haven't flagged eligibility.
  const dispatchState = useMemo(
    () => board?.states.find((s) => s.eligible && !s.terminal)?.name ?? "ready",
    [board],
  );
  const selectedIssues = useMemo(
    () => issues.filter((i) => selectedIds.has(i.id)),
    [issues, selectedIds],
  );
  const allSelectedDispatchable =
    selectedIssues.length > 0 && selectedIssues.every((i) => isDispatchable(i.state));

  const onBulkDispatch = useCallback(async () => {
    const ids = selectedIssues.filter((i) => isDispatchable(i.state)).map((i) => i.id);
    if (ids.length === 0) return;
    // Each dispatch starts a run (cost). Confirm above a small threshold
    // so a fat-fingered select-all + dispatch doesn't fan out a dozen
    // paid runs — pairs with the per-day spend cap.
    if (ids.length > BULK_DISPATCH_CONFIRM_THRESHOLD) {
      if (
        !(await confirm({
          title: `Dispatch ${ids.length} issues?`,
          message: `This starts ${ids.length} runs at once, each consuming budget against the daily spend cap.`,
          confirmLabel: `Dispatch ${ids.length}`,
        }))
      )
        return;
    }
    let queued = 0;
    for (const id of ids) {
      if (await onDrop(id, dispatchState)) queued += 1;
    }
    setSingleSelection(null);
    const plural = (n: number) => (n > 1 ? "s" : "");
    if (queued === ids.length) {
      addToast(`Dispatched ${queued} issue${plural(queued)}`, "success", {
        action: { label: "View runs", onClick: () => setLocation("/runs") },
      });
    } else if (queued > 0) {
      // Partial: some transitions failed (claim conflict, rejected move,
      // tracker error). Surface the gap instead of a misleading all-success
      // toast — the unqueued issues never reached the dispatch lane and the
      // dispatcher will not pick them up.
      addToast(
        `Dispatched ${queued}/${ids.length} issues — ${ids.length - queued} could not be queued`,
        "warning",
        { action: { label: "View runs", onClick: () => setLocation("/runs") } },
      );
    } else {
      addToast(`Could not dispatch ${ids.length} issue${plural(ids.length)}`, "error");
    }
  }, [selectedIssues, dispatchState, onDrop, setSingleSelection, addToast, confirm, setLocation]);

  // runBulkPatch applies a per-issue PATCH across the selection, then
  // refreshes. The patch fn returns null to skip an issue (e.g. a label
  // it already has). Shared by the bulk label / priority / assignee ops.
  const runBulkPatch = useCallback(
    async (build: (iss: NativeIssue) => NativeIssuePatch | null, toastMsg: string) => {
      const targets = selectedIssues;
      if (targets.length === 0) return;
      try {
        let n = 0;
        for (const iss of targets) {
          const patch = build(iss);
          if (patch) {
            await patchIssue(iss.id, patch);
            n++;
          }
        }
        await refresh();
        if (n > 0) addToast(toastMsg, "success");
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [selectedIssues, refresh, addToast],
  );

  const onBulkMove = useCallback(
    async (toState: string) => {
      const ids = selectedIssues.map((i) => i.id);
      if (ids.length === 0) return;
      for (const id of ids) await onDrop(id, toState);
      const display = board?.states.find((s) => s.name === toState)?.display ?? toState;
      addToast(`Moved ${ids.length} to ${display}`, "success");
    },
    [selectedIssues, onDrop, board, addToast],
  );

  const onBulkPriority = useCallback(
    (priority: number) =>
      void runBulkPatch(
        () => ({ priority }),
        `Set priority P${priority} on ${selectedIssues.length} issue${selectedIssues.length > 1 ? "s" : ""}`,
      ),
    [runBulkPatch, selectedIssues],
  );

  const onBulkAssignee = useCallback(
    (assignee: string) =>
      void runBulkPatch(
        () => ({ assignee }),
        assignee
          ? `Assigned ${selectedIssues.length} to @${assignee}`
          : `Cleared assignee on ${selectedIssues.length} issue${selectedIssues.length > 1 ? "s" : ""}`,
      ),
    [runBulkPatch, selectedIssues],
  );

  // Toggle a label across the selection: if every selected issue already
  // has it, remove it from all; otherwise add it to those missing it.
  const onBulkToggleLabel = useCallback(
    (label: string) => {
      const allHave = selectedIssues.length > 0 && selectedIssues.every((i) => (i.labels ?? []).includes(label));
      void runBulkPatch((iss) => {
        const cur = iss.labels ?? [];
        const has = cur.includes(label);
        if (allHave) return has ? { labels: cur.filter((l) => l !== label) } : null;
        return has ? null : { labels: [...cur, label] };
      }, `${allHave ? "Removed" : "Added"} label "${label}" ${allHave ? "from" : "on"} ${selectedIssues.length} issue${selectedIssues.length > 1 ? "s" : ""}`);
    },
    [selectedIssues, runBulkPatch],
  );

  const onBulkDelete = useCallback(async () => {
    const ids = selectedIssues.map((i) => i.id);
    if (ids.length === 0) return;
    if (
      !(await confirm({
        title: `Delete ${ids.length} issue${ids.length > 1 ? "s" : ""}?`,
        message: "This removes them from the board and cannot be undone.",
        confirmLabel: "Delete",
        confirmVariant: "danger",
      }))
    )
      return;
    try {
      for (const id of ids) await deleteIssue(id);
      setSingleSelection(null);
      addToast(`Deleted ${ids.length} issue${ids.length > 1 ? "s" : ""}`, "success");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [selectedIssues, confirm, setSingleSelection, addToast, refresh]);

  // Toggle one card in/out of the multi-selection (keyboard `x` and the
  // per-column select-all share this). The anchor follows the acted-on
  // card so further arrow-key navigation continues from here.
  const toggleSelection = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    setAnchorId(id);
  }, []);
  const selectAllVisible = useCallback(() => {
    const ids = filteredIssues.map((i) => i.id);
    setSelectedIds(new Set(ids));
    setAnchorId(ids[0] ?? null);
  }, [filteredIssues]);
  // Select every card in one state column (per-column select-all box).
  const selectColumn = useCallback(
    (stateName: string) => {
      const ids = (byState.get(stateName) ?? []).map((i) => i.id);
      setSelectedIds((prev) => {
        const next = new Set(prev);
        const allIn = ids.length > 0 && ids.every((id) => next.has(id));
        // Toggle: if the whole column is already selected, clear it;
        // otherwise add the column to the selection.
        for (const id of ids) {
          if (allIn) next.delete(id);
          else next.add(id);
        }
        return next;
      });
      const first = ids[0];
      if (first) setAnchorId(first);
    },
    [byState],
  );

  useBoardKeyboard({
    board,
    byState,
    selectedId: anchorId,
    modalOpen: creating || editing !== null || helpOpen || settingsOpen,
    onSelect: setSingleSelection,
    onToggleSelect: toggleSelection,
    onSelectAllVisible: selectAllVisible,
    onCreate: () => setCreating(true),
    onEdit: (id) => {
      const iss = issues.find((i) => i.id === id);
      if (iss) setEditing(iss);
    },
    onDelete: (id) => void onDelete(id),
    onTransition: (id, toState) => void onDrop(id, toState),
    onShowHelp: () => setHelpOpen((v) => !v),
  });

  useHeaderSlot({
    left: <span className="text-xs font-medium text-fg-default">Board</span>,
    right: board ? (
      <>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => setLocation("/board/labels")}
          title="Manage the board's label vocabulary"
        >
          Labels
        </Button>
        <Button variant="secondary" size="sm" onClick={() => void refresh()}>
          Refresh
        </Button>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          + New issue
        </Button>
      </>
    ) : null,
  });

  if (loading) {
    return <EmptyState message="Loading kanban…" />;
  }
  if (!board) {
    return <EmptyBoard kind="missing" />;
  }

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <DispatcherControlBar onOpenSettings={() => setSettingsOpen(true)} />
      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={() => void refresh()}
      />

      {error && <InlineBanner tone="danger">{error}</InlineBanner>}
      {trackerError && (
        <TrackerErrorBanner
          tracker={trackerError.tracker}
          message={trackerError.message}
        />
      )}
      {dispatcherPaused && (
        <InlineBanner tone="warning" title="Dispatcher paused">
          New issues won't be dispatched until you resume from the toolbar
          above. In-flight runs continue unaffected.
        </InlineBanner>
      )}

      <BoardFilters
        searchQuery={searchQuery}
        labelFilter={labelFilter}
        assigneeFilter={assigneeFilter}
        allLabels={allLabels}
        allAssignees={allAssignees}
        total={issues.length}
        filtered={filteredIssues.length}
        onSearchChange={setSearchQuery}
        onLabelToggle={onLabelToggle}
        onClearLabels={() => setLabelFilter(new Set())}
        onAssigneeChange={setAssigneeFilter}
        sortMode={sortMode}
        onSortChange={setSortMode}
        onReset={() => {
          setSearchQuery("");
          setLabelFilter(new Set());
          setAssigneeFilter("");
        }}
      />

      {issues.length === 0 && (
        <EmptyBoardBanner onCreate={() => setCreating(true)} />
      )}
      {selectedIds.size > 0 && (
        <SelectionToolbar
          count={selectedIds.size}
          board={board}
          allLabels={allLabels}
          allAssignees={allAssignees}
          selectedIssues={selectedIssues}
          allSelectedDispatchable={allSelectedDispatchable}
          onDispatch={() => void onBulkDispatch()}
          onMove={(s) => void onBulkMove(s)}
          onPriority={onBulkPriority}
          onAssignee={onBulkAssignee}
          onToggleLabel={onBulkToggleLabel}
          onDelete={() => void onBulkDelete()}
          onClear={() => setSingleSelection(null)}
        />
      )}
      <div
        className="flex-1 overflow-auto p-3"
        // Click in the empty board area (column gaps, "drop here" space,
        // padding) clears the selection. Clicks landing on a card are
        // ignored here — the card carries data-issue-card and runs its
        // own selection handler.
        onClick={(e) => {
          if ((e.target as HTMLElement).closest("[data-issue-card]")) return;
          if (selectedIds.size > 0) setSingleSelection(null);
        }}
      >
        <div className="flex gap-3 min-w-fit">
          {board.states.map((s) => (
            <Column
              key={s.name}
              name={s.name}
              display={s.display ?? s.name}
              terminal={!!s.terminal}
              eligible={!!s.eligible}
              color={s.color ?? defaultStateColor(s.name, !!s.eligible, !!s.terminal)}
              issues={byState.get(s.name) ?? []}
              selectedIds={selectedIds}
              runningByIssue={runningByIssue}
              retryingByIssue={retryingByIssue}
              skipByIssue={skipByIssue}
              onDrop={onColumnDrop}
              onClickCard={onCardClick}
              onDragStartCard={onCardDragStart}
              onOpenCard={(iss) => setEditing(iss)}
              onSelectColumn={selectColumn}
              onLabelClick={onLabelToggle}
              activeLabels={labelFilter}
              onCancelRun={onCancelRun}
              onOpenRun={(runId) => setLocation(`/runs/${encodeURIComponent(runId)}`)}
              dimmed={dispatcherPaused}
            />
          ))}
          {(byState.get("__unmapped__")?.length ?? 0) > 0 && (
            <Column
              name="__unmapped__"
              display="Unmapped"
              terminal={false}
              eligible={false}
              color="var(--color-board-backlog)"
              issues={byState.get("__unmapped__") ?? []}
              selectedIds={selectedIds}
              runningByIssue={runningByIssue}
              retryingByIssue={retryingByIssue}
              skipByIssue={skipByIssue}
              onDrop={onColumnDrop}
              onClickCard={onCardClick}
              onDragStartCard={onCardDragStart}
              onOpenCard={(iss) => setEditing(iss)}
              onSelectColumn={selectColumn}
              onLabelClick={onLabelToggle}
              activeLabels={labelFilter}
              onCancelRun={onCancelRun}
              onOpenRun={(runId) => setLocation(`/runs/${encodeURIComponent(runId)}`)}
              dimmed={dispatcherPaused}
            />
          )}
        </div>
      </div>

      {creating && (
        <IssueModal
          board={board}
          initial={null}
          onSubmit={onCreate}
          onClose={() => setCreating(false)}
        />
      )}
      {editing && (
        <IssueModal
          board={board}
          initial={editing}
          onSubmit={onSave}
          onClose={() => setEditing(null)}
          onDelete={() => void onDelete(editing.id)}
          onDispatch={
            isDispatchable(editing.state)
              ? () => {
                  const id = editing.id;
                  setEditing(null);
                  void onDrop(id, dispatchState);
                  addToast("Dispatched 1 issue", "success");
                }
              : undefined
          }
        />
      )}
      {confirmDialog}
      {helpOpen && <BoardKeyboardHelp onClose={() => setHelpOpen(false)} />}
    </div>
  );
}

// EmptyBoard renders the "tracker not initialised on disk yet" guide.
// The "board exists but has no issues" case is handled inline above the
// columns by EmptyBoardBanner so the column headers stay visible.
function EmptyBoard({ kind }: { kind: "missing" }) {
  if (kind === "missing") {
    return (
      <div className="p-8 max-w-lg mx-auto text-fg-default space-y-4">
        <div className="text-lg font-semibold">Native tracker not initialised</div>
        <p className="text-sm text-fg-muted">
          The board view persists issues under the project's{" "}
          <code className="text-xs bg-surface-2 px-1 rounded">.iterion/dispatcher/native/</code>{" "}
          directory. iterion creates one automatically on first launch.
        </p>
        <div className="text-sm">
          <p className="mb-1 text-fg-default">Start it from the workspace:</p>
          <pre className="bg-surface-2 rounded p-2 text-xs font-mono overflow-x-auto">
            iterion studio --dir &lt;your-project&gt;
          </pre>
        </div>
      </div>
    );
  }
  return null;
}

const EMPTY_BANNER_DISMISSED_KEY = "iterion.board.empty-banner-dismissed";

// EmptyBoardBanner renders a compact, dismissable onboarding strip
// above the column grid when the board has zero issues. Dismissal
// persists across reloads via localStorage so the chrome only nags on
// first encounter; the columns themselves stay visible regardless.
function EmptyBoardBanner({ onCreate }: { onCreate: () => void }) {
  const [dismissed, setDismissed] = useState(() =>
    readBooleanFlag(EMPTY_BANNER_DISMISSED_KEY),
  );
  if (dismissed) return null;
  const dismiss = () => {
    setDismissed(true);
    writeBooleanFlag(EMPTY_BANNER_DISMISSED_KEY, true);
  };
  return (
    <div className="shrink-0 mx-3 mt-3 rounded border border-border-default bg-surface-1 p-3 text-sm text-fg-default flex items-start gap-3">
      <div className="flex-1 min-w-0">
        <div className="font-medium mb-0.5">Your kanban is empty</div>
        <div className="text-fg-muted text-xs leading-relaxed">
          Create your first issue (or press{" "}
          <kbd className="font-mono px-1 rounded bg-surface-2 border border-border-default">c</kbd>
          ) · Issues land in the first <em>eligible</em> column (green dot) ·
          Wire a dispatcher at{" "}
          <code className="text-xs bg-surface-2 px-1 rounded">/dispatcher</code>{" "}
          to auto-run workflows · Press{" "}
          <kbd className="font-mono px-1 rounded bg-surface-2 border border-border-default">?</kbd>{" "}
          for shortcuts
        </div>
      </div>
      <Button variant="primary" size="sm" onClick={onCreate}>
        + Create issue
      </Button>
      <button
        type="button"
        onClick={dismiss}
        className="text-fg-subtle hover:text-fg-default transition-colors leading-none text-lg px-1"
        title="Dismiss"
        aria-label="Dismiss onboarding banner"
      >
        ×
      </button>
    </div>
  );
}

function BoardFilters({
  searchQuery,
  labelFilter,
  assigneeFilter,
  allLabels,
  allAssignees,
  total,
  filtered,
  onSearchChange,
  onLabelToggle,
  onClearLabels,
  onAssigneeChange,
  sortMode,
  onSortChange,
  onReset,
}: {
  searchQuery: string;
  labelFilter: Set<string>;
  assigneeFilter: string;
  allLabels: string[];
  allAssignees: string[];
  total: number;
  filtered: number;
  onSearchChange: (v: string) => void;
  onLabelToggle: (l: string) => void;
  onClearLabels: () => void;
  onAssigneeChange: (v: string) => void;
  sortMode: SortMode;
  onSortChange: (m: SortMode) => void;
  onReset: () => void;
}) {
  const filtersActive =
    searchQuery.trim() !== "" || labelFilter.size > 0 || assigneeFilter !== "";
  return (
    <div className="px-3 py-2 border-b border-border-default bg-surface-1 flex flex-wrap items-center gap-2 text-xs">
      <input
        type="search"
        value={searchQuery}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder="Search title / body / id…"
        className="px-2 py-1 rounded border border-border-default bg-surface-0 text-fg-default text-xs min-w-[200px] flex-shrink-0"
      />
      {allAssignees.length > 0 && (
        <select
          value={assigneeFilter}
          onChange={(e) => onAssigneeChange(e.target.value)}
          className="px-2 py-1 rounded border border-border-default bg-surface-0 text-fg-default text-xs"
        >
          <option value="">All assignees</option>
          {allAssignees.map((a) => (
            <option key={a} value={a}>
              @{a}
            </option>
          ))}
        </select>
      )}
      {allLabels.length > 0 && (
        <LabelFilter
          allLabels={allLabels}
          selected={labelFilter}
          onToggle={onLabelToggle}
          onClear={onClearLabels}
        />
      )}
      <label className="flex items-center gap-1 text-fg-muted">
        Sort
        <select
          value={sortMode}
          onChange={(e) => onSortChange(e.target.value as SortMode)}
          className="px-2 py-1 rounded border border-border-default bg-surface-0 text-fg-default text-xs"
          title="Order cards within each column"
        >
          {SORT_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      <span className="ml-auto text-fg-muted">
        {filtersActive ? `${filtered} / ${total}` : `${total} issue${total === 1 ? "" : "s"}`}
      </span>
      {filtersActive && (
        <button
          type="button"
          onClick={onReset}
          className="text-fg-subtle hover:text-fg-default underline text-[10px]"
        >
          reset
        </button>
      )}
    </div>
  );
}

// LabelFilter is a searchable multi-select popover for the board's label
// vocabulary. It replaces the flat chip strip, which grew unwieldy once
// boards accumulate dozens of labels. Selection is the same `labelFilter`
// Set the card chips toggle, so the two stay in sync.
function LabelFilter({
  allLabels,
  selected,
  onToggle,
  onClear,
}: {
  allLabels: string[];
  selected: Set<string>;
  onToggle: (l: string) => void;
  onClear: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const rootRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? allLabels.filter((l) => l.toLowerCase().includes(q)) : allLabels;
  }, [allLabels, query]);

  const count = selected.size;

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => {
          setOpen((o) => !o);
          setTimeout(() => inputRef.current?.focus(), 0);
        }}
        className={`px-2 py-1 rounded border flex items-center gap-1 ${
          count > 0
            ? "border-accent text-fg-default bg-accent-soft/30"
            : "border-border-default text-fg-muted hover:text-fg-default bg-surface-0"
        }`}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span>Labels</span>
        {count > 0 && (
          <span className="px-1 rounded bg-accent text-fg-onAccent text-[10px]">{count}</span>
        )}
        <span className="text-fg-subtle text-[10px]">▾</span>
      </button>

      {open && (
        <div className="absolute z-30 mt-1 w-64 max-h-80 overflow-hidden rounded-md border border-border-strong bg-surface-0 shadow-lg flex flex-col">
          <div className="p-1 border-b border-border-default shrink-0">
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search labels…"
              className="w-full bg-surface-1 text-fg-default rounded border border-border-default px-2 py-1 text-xs outline-none focus:border-accent"
            />
          </div>
          <ul className="py-1 overflow-auto">
            {filtered.length === 0 && (
              <li className="px-2 py-2 text-xs text-fg-subtle italic">No matches</li>
            )}
            {filtered.map((l) => {
              const active = selected.has(l);
              return (
                <li key={l}>
                  <button
                    type="button"
                    onClick={() => onToggle(l)}
                    className={`w-full text-left px-2 py-1.5 text-xs flex items-center gap-2 hover:bg-surface-1 ${
                      active ? "text-fg-default" : "text-fg-muted"
                    }`}
                  >
                    <span
                      className={`inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border text-[9px] ${
                        active
                          ? "bg-accent border-accent text-fg-onAccent"
                          : "border-border-strong"
                      }`}
                    >
                      {active ? "✓" : ""}
                    </span>
                    <span className="truncate">{l}</span>
                  </button>
                </li>
              );
            })}
          </ul>
          {count > 0 && (
            <div className="p-1 border-t border-border-default shrink-0">
              <button
                type="button"
                onClick={onClear}
                className="w-full text-center text-[11px] text-fg-subtle hover:text-fg-default py-1"
              >
                Clear {count} label{count > 1 ? "s" : ""}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// SelectionToolbar is the action bar shown whenever ≥1 card is selected.
// It hosts the bulk operations (dispatch, move, priority, assignee,
// label, delete) so triage is possible without opening each card.
function SelectionToolbar({
  count,
  board,
  allLabels,
  allAssignees,
  selectedIssues,
  allSelectedDispatchable,
  onDispatch,
  onMove,
  onPriority,
  onAssignee,
  onToggleLabel,
  onDelete,
  onClear,
}: {
  count: number;
  board: NativeBoard;
  allLabels: string[];
  allAssignees: string[];
  selectedIssues: NativeIssue[];
  allSelectedDispatchable: boolean;
  onDispatch: () => void;
  onMove: (state: string) => void;
  onPriority: (p: number) => void;
  onAssignee: (a: string) => void;
  onToggleLabel: (label: string) => void;
  onDelete: () => void;
  onClear: () => void;
}) {
  const selectClass =
    "px-2 py-0.5 rounded border border-border-default bg-surface-0 text-fg-muted hover:text-fg-default";
  return (
    <div className="shrink-0 px-3 py-1.5 border-b border-border-default bg-accent-soft/20 flex flex-wrap items-center gap-2 text-xs text-fg-default">
      <span>
        <strong>{count}</strong> selected
      </span>
      <button
        type="button"
        onClick={onDispatch}
        disabled={!allSelectedDispatchable}
        title={
          allSelectedDispatchable
            ? "Move all selected into the dispatch lane"
            : "All selected cards must be in Inbox or Backlog"
        }
        className="px-2 py-0.5 rounded bg-success text-white hover:bg-success/90 disabled:opacity-40 disabled:cursor-not-allowed"
      >
        ▶ Let's go
      </button>

      <select
        value=""
        onChange={(e) => {
          if (e.target.value) onMove(e.target.value);
        }}
        className={selectClass}
        title="Move all selected to a column"
      >
        <option value="">Move to…</option>
        {board.states.map((s) => (
          <option key={s.name} value={s.name}>
            {s.display ?? s.name}
          </option>
        ))}
      </select>

      <select
        value=""
        onChange={(e) => {
          if (e.target.value !== "") onPriority(Number(e.target.value));
        }}
        className={selectClass}
        title="Set priority on all selected"
      >
        <option value="">Priority…</option>
        {PRIORITY_PRESETS.map((p) => (
          <option key={p} value={p}>
            P{p}
          </option>
        ))}
      </select>

      <select
        value=""
        onChange={(e) => {
          const v = e.target.value;
          if (v === "") return;
          onAssignee(v === "__clear__" ? "" : v);
        }}
        className={selectClass}
        title="Assign all selected"
      >
        <option value="">Assignee…</option>
        <option value="__clear__">(clear)</option>
        {allAssignees.map((a) => (
          <option key={a} value={a}>
            @{a}
          </option>
        ))}
      </select>

      <BulkLabelPopover
        allLabels={allLabels}
        selectedIssues={selectedIssues}
        onToggle={onToggleLabel}
      />

      <div className="ml-auto flex items-center gap-2">
        <button
          type="button"
          onClick={onDelete}
          className="px-2 py-0.5 rounded border border-danger/50 text-danger-fg hover:bg-danger-soft"
        >
          Delete
        </button>
        <button
          type="button"
          onClick={onClear}
          className="text-fg-subtle hover:text-fg-default underline"
        >
          clear
        </button>
      </div>
    </div>
  );
}

// BulkLabelPopover toggles a label across the whole selection. Each row
// is tri-state: ✓ when every selected issue has the label, – when some
// do, empty when none. Clicking adds it to all (or removes from all when
// every selected issue already has it). Stays open for rapid tagging.
function BulkLabelPopover({
  allLabels,
  selectedIssues,
  onToggle,
}: {
  allLabels: string[];
  selectedIssues: NativeIssue[];
  onToggle: (label: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const rootRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? allLabels.filter((l) => l.toLowerCase().includes(q)) : allLabels;
  }, [allLabels, query]);
  const total = selectedIssues.length;
  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => {
          setOpen((o) => !o);
          setTimeout(() => inputRef.current?.focus(), 0);
        }}
        className="px-2 py-0.5 rounded border border-border-default bg-surface-0 text-fg-muted hover:text-fg-default flex items-center gap-1"
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        Label <span className="text-fg-subtle text-[10px]">▾</span>
      </button>
      {open && (
        <div className="absolute z-30 mt-1 w-64 max-h-80 overflow-hidden rounded-md border border-border-strong bg-surface-0 shadow-lg flex flex-col">
          <div className="p-1 border-b border-border-default shrink-0">
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search labels…"
              className="w-full bg-surface-1 text-fg-default rounded border border-border-default px-2 py-1 text-xs outline-none focus:border-accent"
            />
          </div>
          <ul className="py-1 overflow-auto">
            {filtered.length === 0 && (
              <li className="px-2 py-2 text-xs text-fg-subtle italic">No matches</li>
            )}
            {filtered.map((l) => {
              const c = selectedIssues.reduce(
                (n, i) => n + ((i.labels ?? []).includes(l) ? 1 : 0),
                0,
              );
              const mark = c === 0 ? "" : c === total ? "✓" : "–";
              const active = c > 0;
              return (
                <li key={l}>
                  <button
                    type="button"
                    onClick={() => onToggle(l)}
                    className={`w-full text-left px-2 py-1.5 text-xs flex items-center gap-2 hover:bg-surface-1 ${
                      active ? "text-fg-default" : "text-fg-muted"
                    }`}
                  >
                    <span
                      className={`inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border text-[9px] ${
                        active
                          ? "bg-accent border-accent text-fg-onAccent"
                          : "border-border-strong"
                      }`}
                    >
                      {mark}
                    </span>
                    <span className="truncate">{l}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}

function BoardKeyboardHelp({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" || e.key === "?") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-[var(--z-modal)] bg-black/40 flex items-center justify-center"
      onClick={onClose}
    >
      <div
        className="bg-surface-1 border border-border-default rounded shadow-lg p-5 max-w-sm text-sm"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="font-semibold text-fg-default mb-3">
          Keyboard shortcuts
        </div>
        <ul className="space-y-1.5 text-fg-default">
          <ShortcutRow keys="c / n" desc="New issue" />
          <ShortcutRow keys="click" desc="Select issue" />
          <ShortcutRow keys="title / double-click" desc="Open issue" />
          <ShortcutRow keys="Ctrl/⌘+click" desc="Toggle card in selection" />
          <ShortcutRow keys="Shift+click" desc="Extend selection range" />
          <ShortcutRow keys="Ctrl/⌘+A" desc="Select all visible cards" />
          <ShortcutRow keys="x" desc="Toggle selected card" />
          <ShortcutRow keys="drag selection" desc="Move all selected cards" />
          <ShortcutRow keys="↑ ↓" desc="Navigate cards in column" />
          <ShortcutRow keys="← →" desc="Move card to previous/next column" />
          <ShortcutRow keys="Enter / e" desc="Open selected issue" />
          <ShortcutRow keys="Del / Bksp" desc="Delete selected issue" />
          <ShortcutRow keys="Esc" desc="Clear selection or close" />
        </ul>
        <button
          type="button"
          onClick={onClose}
          className="mt-4 text-xs text-fg-subtle hover:text-fg-default"
        >
          Close
        </button>
      </div>
    </div>
  );
}

function ShortcutRow({ keys, desc }: { keys: string; desc: string }) {
  return (
    <li className="flex items-center justify-between gap-4">
      <kbd className="font-mono text-xs px-1.5 py-0.5 rounded bg-surface-2 border border-border-default">
        {keys}
      </kbd>
      <span className="text-fg-muted text-xs">{desc}</span>
    </li>
  );
}

// ---------------------------------------------------------------------------
// Column + Card
// ---------------------------------------------------------------------------

// defaultStateColor maps the conventional native-tracker state names
// (backlog/ready/in_progress/review/done/blocked) to a sensible palette
// so columns are scannable out of the box. Custom states fall back to a
// semantic colour from the eligible/terminal flags; truly unknown states
// get a neutral slate. Custom boards can always override per-state via
// the `color:` field — this helper only fires when `State.Color` is
// empty.
// Custom MIME for drag payloads carrying one or more issue ids
// (JSON-encoded `string[]`). Matches the studio's existing
// `application/iterion-*` convention so external drops can't
// accidentally be interpreted as text/plain.
const DRAG_MIME_ISSUE_IDS = "application/iterion-issue-ids";

// TERMINAL_BOARD_STATES lists the native-tracker state names treated as
// "no more work" for UI purposes. The runtime contract is that any
// state with `terminal: true` in the board config qualifies — but the
// card doesn't carry the board's flag here, so we hard-code the
// canonical names. Keep in sync with the defaults in
// pkg/dispatcher/native/board.go's NewStore (done + blocked + cancelled).
const TERMINAL_BOARD_STATES = new Set(["done", "blocked", "cancelled"]);

function defaultStateColor(name: string, eligible: boolean, terminal: boolean): string {
  switch (name) {
    case "backlog":
      return "var(--color-board-backlog)";
    case "ready":
      return "var(--color-board-ready)";
    case "in_progress":
      return "var(--color-board-in-progress)";
    case "review":
      return "var(--color-board-review)";
    case "done":
      return "var(--color-board-done)";
    case "blocked":
      return "var(--color-board-blocked)";
    default:
      if (terminal) return "var(--color-board-done)";
      if (eligible) return "var(--color-board-ready)";
      return "var(--color-board-backlog)";
  }
}

interface ColumnProps {
  name: string;
  display: string;
  terminal: boolean;
  eligible: boolean;
  // Hex or CSS color string used to tint the column header strip and the
  // count chip. Always provided by the parent — either from State.Color
  // (board config) or from `defaultStateColor()` (semantic fallback).
  color: string;
  issues: NativeIssue[];
  selectedIds: Set<string>;
  runningByIssue: Map<string, RunningView>;
  retryingByIssue: Map<string, RetryView>;
  skipByIssue: Map<string, DispatchSkipView>;
  // onDrop receives the dropped issue ids (one or more, parsed
  // from the dataTransfer payload) and the destination state name.
  onDrop: (ids: string[], toState: string) => void;
  // onClickCard receives the mouse event so the parent can inspect
  // Shift / Ctrl / Meta modifiers to drive multi-select.
  onClickCard: (iss: NativeIssue, e: React.MouseEvent) => void;
  // onDragStartCard lets the parent decide whether to drag just this
  // card or the full multi-selection, and write the appropriate
  // payload into dataTransfer.
  onDragStartCard: (iss: NativeIssue, e: React.DragEvent) => void;
  // onOpenCard opens the modal directly (used by in-card buttons
  // like "retry details" that should always open regardless of any
  // active selection modifier).
  onOpenCard: (iss: NativeIssue) => void;
  // onSelectColumn toggles the whole column in/out of the selection
  // (the header select-all checkbox).
  onSelectColumn: (stateName: string) => void;
  onLabelClick: (label: string) => void;
  activeLabels: Set<string>;
  onCancelRun: (issueID: string) => void;
  onOpenRun: (runId: string) => void;
  // dimmed: tells the column to render at reduced opacity. Used when the
  // dispatcher is paused so eligible columns visually fade — the cards
  // are still draggable, but the user gets a clear "nothing will pick
  // these up" signal.
  dimmed?: boolean;
}

function Column({
  name,
  display,
  terminal,
  eligible,
  color,
  issues,
  selectedIds,
  runningByIssue,
  retryingByIssue,
  skipByIssue,
  onDrop,
  onClickCard,
  onDragStartCard,
  onOpenCard,
  onSelectColumn,
  onLabelClick,
  activeLabels,
  onCancelRun,
  onOpenRun,
  dimmed,
}: ColumnProps) {
  const [dragOver, setDragOver] = useState(false);
  const selCount = issues.reduce((n, i) => n + (selectedIds.has(i.id) ? 1 : 0), 0);
  const allSelected = issues.length > 0 && selCount === issues.length;
  // Dim only the eligible columns when the dispatcher is paused — the
  // terminal / backlog columns aren't being actively dispatched even
  // when the dispatcher runs, so muting them carries no extra signal.
  const fadeForPause = dimmed && eligible;
  return (
    <div
      className={`w-72 shrink-0 rounded border-2 transition-colors ${
        dragOver
          ? "border-accent bg-accent-soft/30 ring-2 ring-accent/40"
          : "border-border-default bg-surface-1"
      } flex flex-col ${fadeForPause ? "opacity-60" : ""}`}
      style={{ borderTopColor: color, borderTopWidth: 3 }}
      onDragOver={(e) => {
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragOver(false);
        if (name === "__unmapped__") return;
        const json = e.dataTransfer.getData(DRAG_MIME_ISSUE_IDS);
        if (json) {
          try {
            const ids = JSON.parse(json) as unknown;
            if (Array.isArray(ids) && ids.every((x) => typeof x === "string") && ids.length > 0) {
              onDrop(ids as string[], name);
              return;
            }
          } catch {
            // malformed payload — fall through to text/plain
          }
        }
        const single = e.dataTransfer.getData("text/plain");
        if (single) onDrop([single], name);
      }}
    >
      <div className="px-3 py-2 border-b border-border-default flex items-center justify-between text-xs">
        <span className="flex items-center gap-2 min-w-0">
          {name !== "__unmapped__" && issues.length > 0 && (
            <input
              type="checkbox"
              checked={allSelected}
              ref={(el) => {
                if (el) el.indeterminate = selCount > 0 && !allSelected;
              }}
              onChange={() => onSelectColumn(name)}
              title={allSelected ? "Deselect all in column" : "Select all in column"}
              className="shrink-0 accent-accent cursor-pointer"
            />
          )}
          <span
            className="inline-block h-2 w-2 rounded-full shrink-0"
            style={{ backgroundColor: color }}
            aria-hidden="true"
          />
          <span className="font-semibold uppercase tracking-wide text-fg-default truncate">
            {display}
          </span>
        </span>
        <span className="text-fg-muted flex items-center gap-1">
          {selCount > 0 && (
            <span className="text-accent font-medium">{selCount} sel ·</span>
          )}
          {issues.length}
          {eligible && <span className="ml-1 text-success">●</span>}
          {terminal && <span className="ml-1 text-fg-muted">✓</span>}
        </span>
      </div>
      <div className="p-2 flex-1 flex flex-col gap-2 overflow-auto">
        {issues.map((iss) => (
          <IssueCard
            key={iss.id}
            iss={iss}
            selected={selectedIds.has(iss.id)}
            running={runningByIssue.get(iss.id)}
            retrying={retryingByIssue.get(iss.id)}
            skip={skipByIssue.get(iss.id)}
            activeLabels={activeLabels}
            onClick={(e) => onClickCard(iss, e)}
            onOpen={() => onOpenCard(iss)}
            onDragStart={(e) => onDragStartCard(iss, e)}
            onLabelClick={onLabelClick}
            onCancelRun={() => onCancelRun(iss.id)}
            onOpenRun={onOpenRun}
            onShowRetryDetails={() => onOpenCard(iss)}
          />
        ))}
        {issues.length === 0 && (
          <div className="text-xs text-fg-muted text-center py-4">drop here</div>
        )}
      </div>
    </div>
  );
}

interface IssueCardProps {
  iss: NativeIssue;
  selected: boolean;
  running?: RunningView;
  retrying?: RetryView;
  // skip: present when the dispatcher refused to claim this eligible
  // issue because its explicit `bot` is unresolvable / unrouteable.
  // Rendered as a warning badge so the stall is visible + actionable.
  skip?: DispatchSkipView;
  // activeLabels: the set of labels currently in the board-level
  // filter, so each card's label chip can show its active state and
  // operators can see which chips already filter the view.
  activeLabels: Set<string>;
  // onClick receives the mouse event so the parent can update the
  // selection (plain click = select; Shift / Ctrl / Meta = multi-select).
  onClick: (e: React.MouseEvent) => void;
  // onOpen opens the issue modal — triggered by a double-click on the
  // card or a plain click on the title text (GitHub-style).
  onOpen: () => void;
  // onDragStart receives the drag event so the parent can decide
  // whether to drag this card alone or the whole multi-selection
  // and write the right payload into dataTransfer.
  onDragStart: (e: React.DragEvent) => void;
  onLabelClick: (label: string) => void;
  onCancelRun: () => void;
  onOpenRun: (runId: string) => void;
  onShowRetryDetails: () => void;
}

function IssueCard({
  iss,
  selected,
  running,
  retrying,
  skip,
  activeLabels,
  onClick,
  onOpen,
  onDragStart,
  onLabelClick,
  onCancelRun,
  onOpenRun,
  onShowRetryDetails,
}: IssueCardProps) {
  // Hover preview: synthesise a multi-line title combining body
  // (truncated) + key fields + blocker count so the OS-native tooltip
  // provides a quick peek without forcing a modal open. Title strings
  // render with newlines on all major browsers.
  const previewLines: string[] = [];
  if (iss.body) {
    const trimmed = iss.body.trim();
    previewLines.push(trimmed.length > 240 ? trimmed.slice(0, 237) + "…" : trimmed);
  }
  if (iss.fields && Object.keys(iss.fields).length > 0) {
    previewLines.push(
      Object.entries(iss.fields)
        .map(([k, v]) => `${k}: ${String(v)}`)
        .join("\n"),
    );
  }
  if ((iss.blockers?.length ?? 0) > 0) {
    previewLines.push(`Blocked by: ${iss.blockers!.join(", ")}`);
  }
  const hoverTitle = previewLines.length > 0 ? previewLines.join("\n\n") : undefined;
  const [dragging, setDragging] = useState(false);
  const pinnedFields = iss.fields ? pickPinnedFields(iss.fields) : [];
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={iss.title}
      draggable
      data-issue-card
      title={hoverTitle}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
      onDragStart={(e) => {
        onDragStart(e);
        setDragging(true);
      }}
      onDragEnd={() => setDragging(false)}
      onClick={onClick}
      onDoubleClick={onOpen}
      className={`bg-surface-0 border rounded p-2 text-sm cursor-grab active:cursor-grabbing transition-transform ${
        dragging ? "scale-[1.02] shadow-lg" : ""
      } ${
        selected
          ? "border-accent ring-1 ring-accent/40"
          : "border-border-default hover:border-accent/40"
      }`}
    >
      <div className="flex items-start gap-2">
        <span
          // GitHub-style: the title text is the affordance that opens
          // the modal. A plain click here opens; a modified click falls
          // through to the card's selection handler for multi-select.
          className="text-fg-default flex-1 cursor-pointer hover:underline"
          onClick={(e) => {
            if (e.ctrlKey || e.metaKey || e.shiftKey) return;
            e.stopPropagation();
            onOpen();
          }}
        >
          {iss.title}
        </span>
        {iss.priority && iss.priority > 0 ? (
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-warning-soft text-warning-fg">
            P{iss.priority}
          </span>
        ) : null}
      </div>
      {pinnedFields.length > 0 && (
        <div className="mt-0.5 flex items-center gap-2 text-[10px] text-fg-subtle flex-wrap">
          {pinnedFields.map(([k, v]) => (
            <span key={k} className="flex items-center gap-1">
              <span className="font-mono opacity-70">{k}:</span>
              <span className="text-fg-default">{String(v)}</span>
            </span>
          ))}
        </div>
      )}
      {(iss.labels?.length ?? 0) > 0 && (
        <div className="mt-1 flex flex-wrap gap-1">
          {iss.labels!.slice(0, MAX_CARD_LABELS).map((l) => {
            const palette = labelPalette(l);
            const active = activeLabels.has(l);
            return (
              <button
                key={l}
                type="button"
                // Stop propagation so a chip click only toggles the
                // board's label filter — without this the card's
                // onClick would also open the issue modal, which is
                // not what the operator asked for.
                onClick={(e) => {
                  e.stopPropagation();
                  onLabelClick(l);
                }}
                className={`text-[10px] px-1.5 py-0.5 rounded hover:ring-1 hover:ring-accent transition ${
                  active ? "ring-1 ring-accent" : ""
                }`}
                style={palette}
                title={
                  active
                    ? `Click to remove ${l} from the board filter`
                    : `Click to filter board by ${l}`
                }
              >
                {l}
              </button>
            );
          })}
          {iss.labels!.length > MAX_CARD_LABELS && (
            <span
              className="text-[10px] px-1.5 py-0.5 rounded bg-surface-2 text-fg-subtle"
              title={iss.labels!.slice(MAX_CARD_LABELS).join(", ")}
            >
              +{iss.labels!.length - MAX_CARD_LABELS}
            </span>
          )}
        </div>
      )}
      <div className="mt-1 flex items-center gap-2 text-[10px] text-fg-muted flex-wrap">
        <code className="opacity-70">{shortID(iss.id)}</code>
        {iss.bot && (
          <span
            className="font-mono bg-accent/15 text-accent rounded px-1 py-0.5"
            title={`Will dispatch via ${iss.bot} (overrides dispatcher config)`}
          >
            🤖 {iss.bot}
          </span>
        )}
        {iss.assignee && <span>@{iss.assignee}</span>}
        {iss.claim && (
          <span
            className="text-warning-fg"
            title={`Locked by ${iss.claim} — the dispatcher holds the claim until the run finishes.`}
          >
            claimed by {iss.claim}
          </span>
        )}
        {!running && iss.last_run_id && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onOpenRun(iss.last_run_id!);
            }}
            className="font-mono text-info hover:underline opacity-80"
            title={`Open the last run on this issue (run ${iss.last_run_id})`}
          >
            ↪ last run
          </button>
        )}
        {iss.updated_at && (
          <span className="text-fg-subtle" title={iss.updated_at}>
            · updated {formatRelative(iss.updated_at)}
          </span>
        )}
      </div>
      {running && (
        <div className="mt-1 flex items-center justify-between gap-2 rounded bg-success-soft px-1.5 py-1 text-[10px] text-success-fg">
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onOpenRun(running.run_id);
            }}
            className="text-left flex-1 hover:underline cursor-pointer"
            title={
              running.attempt && running.attempt > 0
                ? `Open run ${running.run_id} (resume of a prior failed_resumable run — attempt ${running.attempt + 1})`
                : `Open run ${running.run_id}`
            }
          >
            ● {running.attempt && running.attempt > 0 ? "resuming" : "running"}
            {running.attempt && running.attempt > 0 ? (
              <span className="ml-1 text-warning-fg/90">#{running.attempt + 1}</span>
            ) : null}
            {running.last_event_name && (
              <span className="ml-1 text-success-fg/70">— {running.last_event_name}</span>
            )}
          </button>
          <button
            className="rounded border border-success/40 px-1.5 py-0.5 text-[10px] hover:bg-success-soft"
            onClick={(e) => {
              e.stopPropagation();
              onCancelRun();
            }}
            title="Cancel this in-flight run"
          >
            cancel
          </button>
        </div>
      )}
      {!running && retrying && !TERMINAL_BOARD_STATES.has(iss.state) && (
        <button
          type="button"
          className="mt-1 w-full text-left rounded bg-warning-soft px-1.5 py-1 text-[10px] text-warning-fg cursor-pointer hover:bg-warning-soft"
          onClick={(e) => {
            e.stopPropagation();
            onShowRetryDetails();
          }}
          title={retrying.error ? `Last error: ${retrying.error}` : undefined}
        >
          ⏳ retrying (attempt {retrying.attempt})
          {retrying.error && (
            <span className="ml-1 text-warning-fg/80 truncate">— {retrying.error}</span>
          )}
        </button>
      )}
      {!running && retrying && TERMINAL_BOARD_STATES.has(iss.state) && (
        <div
          className="mt-1 rounded bg-fg-muted/10 px-1.5 py-1 text-[10px] text-fg-subtle"
          title={`The dispatcher still has a retry entry for this issue, but it's in a terminal state (${iss.state}) — the retry will be skipped on the next tick.`}
        >
          stale retry queued — will be skipped (issue in {iss.state})
        </div>
      )}
      {!running && skip && (
        <button
          type="button"
          className="mt-1 w-full text-left rounded bg-danger-soft px-1.5 py-1 text-[10px] text-danger-fg cursor-pointer hover:bg-danger-soft"
          onClick={(e) => {
            e.stopPropagation();
            onOpen();
          }}
          title={`The dispatcher refuses to run this issue: ${skip.reason}. Fix the bot in the issue editor or add it to assignee_workflows.`}
        >
          ⚠ won&apos;t dispatch
          <span className="ml-1 text-danger-fg/80 truncate">— {skip.reason}</span>
        </button>
      )}
    </div>
  );
}

// labelPalette derives a stable pastel background + foreground colour
// from a label name. Two cards with the label "urgent" always render
// the same colour, but "infra" and "urgent" land on visibly distinct
// palettes. Hashing avoids the need for a label-colour schema in the
// backend — operators get colour scanning today without configuration.
// A small alias table covers common semantic labels with sensible
// presets (red for "urgent" / "bug", green for "ready", etc.).
// Token-driven alias table at module scope: built once, not per-label
// per-card per-render. Severity labels reuse the prebuilt design-system
// *-soft pairs (single source of truth for the 18% tint); docs borrows the
// iteration-1 (purple) hue, which has no -soft token, via softColor. The
// chips invert correctly in light mode because the values are CSS vars.
const DANGER_CHIP = { backgroundColor: "var(--color-danger-soft)", color: "var(--color-danger-fg)" };
const SUCCESS_CHIP = { backgroundColor: "var(--color-success-soft)", color: "var(--color-success-fg)" };
const LABEL_ALIASES: Record<string, { backgroundColor: string; color: string }> = {
  urgent: DANGER_CHIP,
  blocker: DANGER_CHIP,
  bug: DANGER_CHIP,
  infra: { backgroundColor: "var(--color-info-soft)", color: "var(--color-info-fg)" },
  docs: { backgroundColor: softColor("var(--color-iteration-1)", 18), color: "var(--color-fg-default)" },
  feature: SUCCESS_CHIP,
  ready: SUCCESS_CHIP,
};

function labelPalette(label: string): { backgroundColor: string; color: string } {
  const hit = LABEL_ALIASES[label.toLowerCase()];
  if (hit) return hit;
  // Stable 32-bit FNV-1a hash → hue. Fixed S/L keeps the palette readable
  // against both light and dark surfaces.
  let h = 2166136261 >>> 0;
  for (let i = 0; i < label.length; i++) {
    h ^= label.charCodeAt(i);
    h = Math.imul(h, 16777619) >>> 0;
  }
  const hue = h % 360;
  return {
    backgroundColor: `hsl(${hue}, 60%, 28%)`,
    color: `hsl(${hue}, 80%, 88%)`,
  };
}

// pickPinnedFields returns up to two scalar field entries from a card's
// `fields` map so the card body can surface high-signal data (enum
// statuses, customer IDs) inline without expanding the modal. Skips
// fields whose value is too long for a card row — those belong in the
// hover preview / modal view.
function pickPinnedFields(fields: Record<string, unknown>): Array<[string, unknown]> {
  const picked: Array<[string, unknown]> = [];
  for (const [k, v] of Object.entries(fields)) {
    if (picked.length >= 2) break;
    if (v === null || v === undefined) continue;
    if (typeof v === "object") continue;
    const str = String(v);
    if (str.length === 0 || str.length > 32) continue;
    picked.push([k, v]);
  }
  return picked;
}


function shortID(id: string) {
  const bare = id.replace(/^native:/, "").replace(/^github:[^#]+#/, "#");
  return bare.length > 10 ? bare.slice(0, 10) : bare;
}
