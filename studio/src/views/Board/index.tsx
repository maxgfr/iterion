import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearch } from "wouter";

import { readBooleanFlag, writeBooleanFlag } from "@/lib/localStorageFlag";

import { useHeaderSlot } from "@/components/shared/useHeaderSlot";
import DispatcherControlBar from "@/components/shared/DispatcherControlBar";
import { Button } from "@/components/ui/Button";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { EmptyState } from "@/components/ui/EmptyState";
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
import { BoardFilters } from "./BoardFilters";
import { BoardKeyboardHelp } from "./BoardKeyboardHelp";
import { Column } from "./Column";
import { SelectionToolbar } from "./SelectionToolbar";
import SettingsDrawer from "@/components/Dispatcher/SettingsDrawer";
import TrackerErrorBanner from "@/components/shared/TrackerErrorBanner";
import { useBoardKeyboard } from "@/hooks/useBoardKeyboard";
import { useConfirm } from "@/hooks/useConfirm";
import { useToggleSet } from "@/hooks/useToggleSet";
import { useUIStore } from "@/store/ui";
import {
  defaultStateColor,
  DRAG_MIME_ISSUE_IDS,
  type SortMode,
} from "./boardShared";

// Pre-dispatch lanes: an issue here has not yet entered the dispatcher's
// eligible queue, so it can be "dispatched" (transitioned into the
// dispatch lane). review/done/blocked are downstream and not dispatchable.
function isDispatchable(state: string): boolean {
  return state === "inbox" || state === "backlog";
}

// Above this many at once, bulk dispatch asks for confirmation — each
// dispatch starts a paid run.
const BULK_DISPATCH_CONFIRM_THRESHOLD = 3;

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
  const {
    set: labelFilter,
    toggle: onLabelToggle,
    clear: clearLabelFilter,
  } = useToggleSet<string>();
  const [assigneeFilter, setAssigneeFilter] = useState("");
  const [sortMode, setSortMode] = useState<SortMode>("priority");
  // `onLabelToggle` (from useToggleSet) is the single source of truth
  // for label filter toggling — used both by the top filter strip and
  // by clicking a chip on any card, so card-level chips toggle the
  // same Set the filter strip shows.

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
        onClearLabels={clearLabelFilter}
        onAssigneeChange={setAssigneeFilter}
        sortMode={sortMode}
        onSortChange={setSortMode}
        onReset={() => {
          setSearchQuery("");
          clearLabelFilter();
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

