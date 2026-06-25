// E2E smoke loop for the operator's whats-next → board → dispatcher →
// bot → findings-inbox cycle. No LLM calls and no runtime IR execution;
// the whats-next side is represented by a static IR check on the
// emit_action prompt plus boardops calls that mirror what emit_action /
// assign_to_bots / a dispatched bot do, while the board + dispatcher are
// the real native store + actor driven by a StubRunner.
//
// Regression guards bundled into the tests:
//   - commit 89249f02 — emit_action's user prompt MUST reference
//     {{input.selected_titles}} so the LLM-side filter actually fires
//     (TestWhatsNext_EmitAction_UserPromptReferencesSelectedTitles), and
//     the filter contract itself is pinned behaviourally against the
//     board (TestWhatsNext_EmitAction_SelectedTitlesFilterMatrix).
//   - commit 45eafe28 — dispatcher MUST auto-transition in_progress →
//     review on a clean run finish (otherwise the issue stays eligible
//     and gets re-dispatched on the next tick)
//     (TestWhatsNext_Loop_DispatchAutoTransitionsNoReloop).
//   - commit c134af2e — findings moved from PROJECT_MEMORY_DIR/findings/
//     *.md files into board issues in a non-eligible `inbox` state. That
//     change dropped the old .md survival guard; this file re-adds it in
//     board form — a dispatched bot's inbox finding survives the dispatch
//     lifecycle and is itself never dispatched
//     (TestWhatsNext_Loop_FindingsInboxSurvivesDispatch).

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/dispatcher"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native/boardops"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// TestWhatsNext_EmitAction_UserPromptReferencesSelectedTitles guards
// commit 89249f02. The fix surfaced `{{input.selected_titles}}` in the
// emit_action_user prompt so the LLM applies the operator's per-item
// selection BEFORE materialising roadmap items as kanban issues.
// Without that reference the filter silently degrades to "create every
// item" — the failure mode that prompted the 7-vs-5 issue-count
// mismatch caught in the 2026-05-24 dogfood.
func TestWhatsNext_EmitAction_UserPromptReferencesSelectedTitles(t *testing.T) {
	wf := compileFixture(t, "whats-next/main.bot")
	p, ok := wf.Prompts["emit_action_user"]
	if !ok {
		t.Fatal("emit_action_user prompt missing from whats-next/main.bot")
	}
	if !strings.Contains(p.Body, "selected_titles") {
		t.Fatalf("emit_action_user prompt no longer references selected_titles — regression of 89249f02\nprompt body:\n%s", p.Body)
	}
}

// newSmokeDispatcherFixture wires a dispatcher + native store +
// StubRunner the same way newDispatcherFixture does, but routes the
// Config through ApplyDefaults() so CompletedState defaults to "review"
// (matches the production path post-45eafe28). Kept as a sibling helper
// rather than modifying newDispatcherFixture to avoid changing the
// semantics other dispatcher_test.go cases rely on.
func newSmokeDispatcherFixture(t *testing.T, polling time.Duration) (
	*dispatcher.Dispatcher,
	*native.Store,
	*dispatcher.StubRunner,
	func(),
) {
	t.Helper()
	dir := t.TempDir()

	ns, err := native.NewStore(dir + "/dispatcher")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ws, err := dispatcher.NewWorkspaces(dir + "/dispatcher/workspaces")
	if err != nil {
		t.Fatalf("NewWorkspaces: %v", err)
	}

	cfg := &dispatcher.Config{
		Name:      "e2e-smoke-loop",
		Workflow:  dir + "/dummy.bot",
		Tracker:   dispatcher.TrackerConfig{Kind: "native"},
		Polling:   dispatcher.PollingConfig{IntervalMS: int(polling.Milliseconds())},
		Agent:     dispatcher.AgentConfig{MaxConcurrent: 2, MaxRetryBackoffMS: 500},
		Workspace: dispatcher.WorkspaceConfig{Root: dir + "/dispatcher/workspaces"},
	}
	cfg.ApplyDefaults()

	logger := iterlog.New(iterlog.LevelError, &bytes.Buffer{})
	runner := &dispatcher.StubRunner{}
	c, err := dispatcher.New(dispatcher.Options{
		Config:     cfg,
		Tracker:    native.NewAdapter(ns),
		Runner:     runner,
		Workspaces: ws,
		Logger:     logger,
		StoreDir:   dir,
		HostMarker: "e2e-smoke",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	return c, ns, runner, func() { cancel(); c.Stop() }
}

// TestWhatsNext_Loop_DispatchAutoTransitionsNoReloop drives the
// production board → dispatcher loop end-to-end with stubs:
//
//  1. Boot a dispatcher with ApplyDefaults() so CompletedState=review
//     mirrors production.
//  2. Create two ready issues via boardops (matches the production
//     path emit_action takes — boardops.Call create_issue per surviving
//     roadmap item).
//  3. StubRunner clean-finishes each dispatch; the actor must
//     auto-transition the issue in_progress → review (guard 45eafe28).
//  4. Wait several polling intervals and assert the dispatch counter
//     stays at 2 — without 45eafe28 the issues would remain in
//     in_progress + eligible and the actor would re-dispatch them.
//
// Wall-clock budget: dispatch loop finishes in ~3× polling (claim →
// finish → transition), then 5× polling for the no-reloop watch.
// At 50ms polling that's ~400ms; the deadline is 3s for slow CI.
func TestWhatsNext_Loop_DispatchAutoTransitionsNoReloop(t *testing.T) {
	const polling = 50 * time.Millisecond
	c, ns, runner, cleanup := newSmokeDispatcherFixture(t, polling)
	defer cleanup()

	var dispatchCount atomic.Int32
	runner.Handler = func(_ context.Context, _ dispatcher.DispatchSpec) error {
		dispatchCount.Add(1)
		return nil
	}

	caps := boardops.NewCapabilities("board.create,board.read,board.move,board.assign")
	mkIssue := func(title string) native.Issue {
		raw, err := boardops.Call(ns, caps, "create_issue", json.RawMessage(`{"title":"`+title+`","state":"ready","assignee":"feature_dev"}`))
		if err != nil {
			t.Fatalf("create_issue %q: %v", title, err)
		}
		var iss native.Issue
		if err := json.Unmarshal(raw, &iss); err != nil {
			t.Fatalf("unmarshal %q: %v", title, err)
		}
		return iss
	}
	issX := mkIssue("Refactor X")
	issY := mkIssue("Implement Y")

	// Wait for both issues to reach review state.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		xs, _ := ns.Get(issX.ID)
		ys, _ := ns.Get(issY.ID)
		if xs != nil && ys != nil && xs.State == native.StateReview && ys.State == native.StateReview {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	finalX, _ := ns.Get(issX.ID)
	finalY, _ := ns.Get(issY.ID)
	if finalX == nil || finalX.State != native.StateReview {
		t.Fatalf("issue %q never reached review — got state=%v (guard 45eafe28)", issX.Title, stateOf(finalX))
	}
	if finalY == nil || finalY.State != native.StateReview {
		t.Fatalf("issue %q never reached review — got state=%v (guard 45eafe28)", issY.Title, stateOf(finalY))
	}
	if got := dispatchCount.Load(); got != 2 {
		t.Fatalf("expected exactly 2 dispatches before transition, got %d", got)
	}

	// No-reloop guard: dispatcher must not re-dispatch issues sitting
	// in review. Wait several polling intervals; counter must stay at 2.
	time.Sleep(5 * polling)
	if got := dispatchCount.Load(); got != 2 {
		t.Fatalf("re-dispatch detected after review transition: dispatchCount=%d (expected 2) — regression of 45eafe28", got)
	}
	if running := len(c.Snapshot().Running); running != 0 {
		t.Fatalf("running set not drained after clean finish: %d still running", running)
	}
}

func stateOf(iss *native.Issue) string {
	if iss == nil {
		return "<nil>"
	}
	return iss.State
}

// titlesOf collects issue titles for readable failure messages.
func titlesOf(issues []*native.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, iss := range issues {
		out = append(out, iss.Title)
	}
	return out
}

// applySelectedTitles mirrors emit_action_system step 0: the operator's
// per-item selection filter, applied BEFORE materialising roadmap items
// as kanban issues. Encoding the prose contract as executable Go pins the
// behaviour the LLM is instructed to follow; the static prompt guard
// (TestWhatsNext_EmitAction_UserPromptReferencesSelectedTitles) is the
// complementary check that the real emit_action_user prompt still carries
// that instruction. There is no Go-side production filter to call here —
// the filtering is performed by the LLM per the prompt — so this helper
// is the executable spec, exercised by TestWhatsNext_EmitAction_SelectedTitlesFilterMatrix.
//
// Contract (see bots/whats-next/main.bot, emit_action_system step 0):
//   - empty / nil / ["all"] (case-insensitive) → keep EVERY item
//     (default approve-all behaviour).
//   - otherwise → keep only items whose title exactly matches an entry in
//     the selection; unmatched selection entries are dropped silently.
func applySelectedTitles(items, selected []string) []string {
	if len(selected) == 0 {
		return slices.Clone(items)
	}
	if len(selected) == 1 && strings.EqualFold(strings.TrimSpace(selected[0]), "all") {
		return slices.Clone(items)
	}
	want := make(map[string]bool, len(selected))
	for _, s := range selected {
		want[s] = true
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if want[it] {
			out = append(out, it)
		}
	}
	return out
}

// TestWhatsNext_EmitAction_SelectedTitlesFilterMatrix pins the behavioural
// contract of emit_action's per-item selection (commit 89249f02) at the
// board level: given a proposed roadmap and the operator's selection,
// only the surviving items become kanban issues. The static prompt guard
// proves the LLM is told to filter; this proves the downstream board ends
// up with exactly the selected subset (and that the empty / "all" default
// still materialises everything). No dispatcher — pure boardops, ~instant.
func TestWhatsNext_EmitAction_SelectedTitlesFilterMatrix(t *testing.T) {
	roadmap := []string{"Refactor X", "Implement Y", "Document Z"}
	caps := boardops.NewCapabilities("board.create,board.read")

	cases := []struct {
		name     string
		selected []string
		want     []string
	}{
		{"nil selection → approve-all", nil, roadmap},
		{"empty selection → approve-all", []string{}, roadmap},
		{"all sentinel → approve-all", []string{"all"}, roadmap},
		{"ALL sentinel is case-insensitive", []string{"ALL"}, roadmap},
		{"explicit subset", []string{"Refactor X", "Document Z"}, []string{"Refactor X", "Document Z"}},
		{"subset drops unmatched selection entry", []string{"Refactor X", "Nonexistent"}, []string{"Refactor X"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns, err := native.NewStore(t.TempDir() + "/dispatcher")
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}

			// emit_action materialises only the surviving items as
			// `backlog` issues (state per emit_action_system step 3a).
			for _, title := range applySelectedTitles(roadmap, tc.selected) {
				args, _ := json.Marshal(map[string]any{"title": title, "state": native.StateBacklog})
				if _, err := boardops.Call(ns, caps, "create_issue", args); err != nil {
					t.Fatalf("create_issue %q: %v", title, err)
				}
			}

			list, err := ns.List(native.ListFilter{States: []string{native.StateBacklog}})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			got := make(map[string]bool, len(list))
			for _, iss := range list {
				got[iss.Title] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("created %d issues %v, want %d %v", len(list), titlesOf(list), len(tc.want), tc.want)
			}
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("expected issue %q on board, missing; board has %v", w, titlesOf(list))
				}
			}
		})
	}
}

// TestWhatsNext_Loop_FindingsInboxSurvivesDispatch drives the full
// board → dispatcher → bot → findings-inbox loop with stubs and asserts
// the findings dimension dropped in commit c134af2e (when findings moved
// from .md files to the board's `inbox` state) now holds as a board
// invariant:
//
//  1. Two `ready` work issues are created (mirrors assign_to_bots having
//     promoted operator-selected backlog items to ready).
//  2. The dispatcher claims each (ready → in_progress) and runs the bot.
//  3. Each dispatched bot records ONE out-of-scope observation into the
//     non-eligible `inbox` state (the findings flow), then clean-finishes.
//  4. The dispatcher auto-transitions each WORK issue in_progress → review
//     (guard 45eafe28) WITHOUT touching the inbox findings.
//  5. The inbox findings survive — same count, still in `inbox`, never
//     claimed, `findings` label intact — and are never themselves
//     dispatched (inbox is non-eligible), so the dispatch count holds at 2
//     across several further polls (no re-dispatch loop).
//
// Wall-clock budget: ~3× polling for dispatch→finish→transition, then
// 5× polling for the no-reloop watch. At 50ms polling that's well under
// the 3s deadline used for slow CI.
func TestWhatsNext_Loop_FindingsInboxSurvivesDispatch(t *testing.T) {
	const polling = 50 * time.Millisecond
	c, ns, runner, cleanup := newSmokeDispatcherFixture(t, polling)
	defer cleanup()

	botCaps := boardops.NewCapabilities("board.read,board.create")
	var dispatchCount atomic.Int32

	// Each dispatched bot posts one finding to the board's `inbox` state
	// (the c134af2e flow: create_issue state=inbox + `findings` label),
	// then signals a clean finish. It does NOT move its own work issue —
	// that's the dispatcher's job (and moving it would suppress the
	// auto-transition, cf. dispatcher's SkipsAutoTransitionWhenWorkflowMovedState).
	runner.Handler = func(_ context.Context, _ dispatcher.DispatchSpec) error {
		n := dispatchCount.Add(1)
		finding, _ := json.Marshal(map[string]any{
			"title":  fmt.Sprintf("Out-of-scope finding %d", n),
			"state":  native.StateInbox,
			"labels": []string{"findings", "kind:bug", "source:feature_dev"},
		})
		if _, err := boardops.Call(ns, botCaps, "create_issue", finding); err != nil {
			t.Errorf("bot posting finding to inbox: %v", err) // Errorf: safe off the test goroutine
		}
		return nil
	}

	caps := boardops.NewCapabilities("board.create,board.read,board.move,board.assign")
	mkReady := func(title string) native.Issue {
		raw, err := boardops.Call(ns, caps, "create_issue", json.RawMessage(`{"title":"`+title+`","state":"ready","assignee":"feature_dev"}`))
		if err != nil {
			t.Fatalf("create_issue %q: %v", title, err)
		}
		var iss native.Issue
		if err := json.Unmarshal(raw, &iss); err != nil {
			t.Fatalf("unmarshal %q: %v", title, err)
		}
		return iss
	}
	issX := mkReady("Refactor X")
	issY := mkReady("Implement Y")

	// Wait for both work issues to auto-transition to review.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		xs, _ := ns.Get(issX.ID)
		ys, _ := ns.Get(issY.ID)
		if xs != nil && ys != nil && xs.State == native.StateReview && ys.State == native.StateReview {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	finalX, _ := ns.Get(issX.ID)
	finalY, _ := ns.Get(issY.ID)
	if finalX == nil || finalX.State != native.StateReview {
		t.Fatalf("work issue %q never reached review — got state=%v (guard 45eafe28)", issX.Title, stateOf(finalX))
	}
	if finalY == nil || finalY.State != native.StateReview {
		t.Fatalf("work issue %q never reached review — got state=%v (guard 45eafe28)", issY.Title, stateOf(finalY))
	}
	if got := dispatchCount.Load(); got != 2 {
		t.Fatalf("expected exactly 2 work dispatches, got %d", got)
	}

	// Findings survive: both inbox issues persist untouched, with the
	// `findings` label, never claimed (inbox is non-eligible).
	inbox, err := ns.List(native.ListFilter{States: []string{native.StateInbox}})
	if err != nil {
		t.Fatalf("List inbox: %v", err)
	}
	if len(inbox) != 2 {
		t.Fatalf("expected 2 surviving inbox findings, got %d: %v", len(inbox), titlesOf(inbox))
	}
	for _, f := range inbox {
		if f.State != native.StateInbox {
			t.Errorf("finding %q drifted out of inbox: state=%s", f.Title, f.State)
		}
		if f.Claim != "" {
			t.Errorf("finding %q was claimed (%q) — inbox must never be dispatched", f.Title, f.Claim)
		}
		if !slices.Contains(f.Labels, "findings") {
			t.Errorf("finding %q lost its `findings` label: %v", f.Title, f.Labels)
		}
	}

	// No re-dispatch: inbox is non-eligible and review is not eligible, so
	// several more polls must not grow the dispatch count.
	time.Sleep(5 * polling)
	if got := dispatchCount.Load(); got != 2 {
		t.Fatalf("re-dispatch detected: dispatchCount=%d (expected 2) — inbox findings or review issues were re-picked", got)
	}
	if running := len(c.Snapshot().Running); running != 0 {
		t.Fatalf("running set not drained after clean finish: %d still running", running)
	}
}
