# ADR-007: Run-view files panel — combined default by lifecycle, merged from two git queries

- **Status**: Accepted
- **Date**: 2026-05-29
- **Authors**: devthejo
- **Code**: [pkg/server/runs_files.go](../../pkg/server/runs_files.go)
  (`modeCombined`, `combinedFiles`, lifecycle tags),
  [pkg/git/git.go](../../pkg/git/git.go) (`FileStatus.Lifecycle`),
  [studio/src/api/runs.ts](../../studio/src/api/runs.ts)
  (`mergeActionReady`, `smartDefaultFilesMode`, `RunFilesMode`),
  [studio/src/components/Runs/FilesPanel.tsx](../../studio/src/components/Runs/FilesPanel.tsx)
  (reactive default, lifecycle tint),
  [studio/src/components/Runs/LeftPanel.tsx](../../studio/src/components/Runs/LeftPanel.tsx)

## Context

The run-view left files/diff panel had two scopes: `uncommitted` (`git
status` against the worktree) and `branch` (`BaseCommit..HEAD` range). The
default scope was the user's last segmented-control pick, **persisted
globally to `localStorage`** (`run-console-v1.files-mode`), independent of
which run or what phase it was in.

The ask: default the panel to what the operator most likely wants at each
phase. **While a run is in progress** — show a *combined* view of all
branch commits **plus** the uncommitted working-tree changes, with a subtle
per-file committed-vs-uncommitted distinction. **Once the run finishes and
the "Squash & merge" button is shown** — flip the default to the committed
branch diff (what would actually merge). The flip must be **reactive** to
state transitions, and all existing modes must stay selectable.

Two facts shaped the design:

1. There was **no "combined" range** anywhere. A single `git diff
   BaseCommit` (worktree vs. base) would roll committed + uncommitted into
   one diff but **cannot tell you, per file, which is which** — and the
   feature explicitly needs that distinction.
2. The committed-vs-uncommitted split is exactly what the two existing,
   battle-tested helpers already compute: `StatusBetween(base..HEAD)` and
   `Status()` (each with numstat + untracked handling, parallelized).

## Decision

### 1. Combined = merge of the two existing queries, 2-state lifecycle

`combinedFiles` (server) unions `Status()` (tagged `uncommitted`) with
`StatusBetween(BaseCommit..HEAD)` (tagged `committed`), **uncommitted wins
on a path collision**, sorted by path. The tag rides on a new optional
`gitlib.FileStatus.Lifecycle` field (`omitempty` → every other mode's wire
shape is byte-identical). No new git primitive; no new diff endpoint — a
combined row forwards `branch` or `uncommitted` to the existing
`/files/diff` based on its lifecycle, so the per-row diff matches the
per-row line counts.

The deliberate concession: a **"mixed" file** (committed during the run
*and* re-edited) is shown **once**, tagged `uncommitted`, with its
still-pending (`HEAD..worktree`) line counts — not a `base..worktree`
total. We do **not** synthesize a third "mixed" state.

### 2. Reactive lifecycle default, replacing global persistence

The FilesPanel default is now `smartDefaultFilesMode(mergeReady)` —
`combined` while in flight, `branch` once `mergeActionReady(run)` is true.
`mergeActionReady` mirrors the Commits panel's `mergeable && hasBranch`
gate (terminal status + `final_branch`), so "default flips" and "merge
button appears" are the same signal. An explicit operator pick is held in
an **ephemeral, per-run** `userMode` override (reset on `runId` change);
the **global `localStorage` mode persistence was removed**.

## Trade-offs

| Dimension | Merge two existing queries, 2-state (chosen) | New `base..worktree` helper, 3-state "mixed" |
|---|---|---|
| Git helpers added | none — reuses `Status` + `StatusBetween` | new name-status+numstat-against-worktree + untracked union |
| Per-file provenance | ✅ committed/uncommitted from set membership | ✅, but needs `git status` cross-ref anyway |
| Mixed-file line counts | shows pending delta only (documented) | true `base..worktree` total |
| Diff endpoint | unchanged (rows forward branch/uncommitted) | needs a combined diff range (`base..worktree`) |
| Row count ↔ row diff consistency | ✅ same source per row | ✅ |
| Risk / surface | low — two vetted functions | higher — new git code + tests |

| Dimension | Reactive lifecycle default (chosen) | Keep global localStorage default |
|---|---|---|
| "Default follows the run" | ✅ by construction | ✗ a stale pick (e.g. `branch`) overrides every new run |
| Manual selection respected | ✅ per-run override | ✅ but sticky across runs |
| Cross-run memory of a pick | ✗ removed | ✅ |

## Alternatives considered

1. **Single `git diff BaseCommit` for the combined list.** Rejected: rolls
   committed + uncommitted into one diff with **no per-file provenance**,
   which is the feature's core requirement.
2. **3-state `mixed` + `base..worktree` counts.** Rejected for v1: needs a
   new git helper and a combined diff range for marginal value on a rare
   case. The 2-state model matches the literal "committed vs uncommitted"
   ask; "in `git status` ⇒ in-flight" is the most useful bucket for a mixed
   file. Named as a future enhancement.
3. **Backend default = combined.** Rejected: the lifecycle signal lives in
   the run snapshot the frontend already holds and updates reactively; the
   frontend always sends an explicit mode, so the backend's `""`→
   `uncommitted` fallback is left untouched (only direct API callers hit
   it).
4. **Keep global localStorage persistence, let smart default win on
   transition.** Rejected: more state to reconcile, and it reintroduces the
   "every run opens in the last-picked scope" bug the feature exists to
   fix. An ephemeral per-run override is simpler and matches "this only
   changes the DEFAULT selection."

## Consequences

- **In-progress runs default to the full picture** (committed + pending),
  with a faint warm tint on uncommitted rows and a cool tint on committed
  rows (plus tooltip/aria); finished runs default to the merge preview.
- **The flip is automatic.** `mergeReady` derives from the run snapshot
  (WS/poll-updated) and is a prop, so a finishing run recomputes the mode
  and `useRunFiles` refetches — no one-time init.
- **All three modes stay selectable**; `worktree_gone` disables
  uncommitted + combined and auto-falls-back to branch (backend reports the
  reason for both, frontend pins the override to branch).
- **A picked mode no longer persists across runs.** Operators who relied on
  "always open in branch" lose that; the trade was accepted to make the
  lifecycle default authoritative.
- **Mixed files under-report.** A committed-then-re-edited file shows its
  pending delta, not its total since base — documented in `combinedFiles`;
  the per-row diff stays consistent with that number.
- **Combined runs two git scans** (`Status` + `StatusBetween`) per fetch
  vs. one; both are already parallelized internally. Acceptable; noted for
  very large worktrees.
