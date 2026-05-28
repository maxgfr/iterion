package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/dispatcher/tracker"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// lastRunCall captures one SetLastRun invocation so the test can
// assert on the (id, run, workdir) trio without coupling to the
// native package.
type lastRunCall struct {
	id      string
	runID   string
	workdir string
}

// lastRunTracker wraps fakeTracker and implements the optional
// SetLastRun shape the dispatcher type-asserts to. Records each call
// so the test can assert ordering + content.
type lastRunTracker struct {
	*fakeTracker

	mu    sync.Mutex
	calls []lastRunCall
}

func newLastRunTracker() *lastRunTracker {
	return &lastRunTracker{fakeTracker: newFakeTracker()}
}

func (t *lastRunTracker) SetLastRun(id, runID, workdir string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, lastRunCall{id: id, runID: runID, workdir: workdir})
	return nil
}

func (t *lastRunTracker) lastRunCalls() []lastRunCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]lastRunCall, len(t.calls))
	copy(out, t.calls)
	return out
}

// TestFinishRun_StampsLastRunOnNativeTracker exercises the stamp path
// via the type-assertion: the fake implements SetLastRun, the
// dispatcher's finishRun must call it with the right (issue, run,
// workdir) tuple regardless of whether the run finished cleanly or
// failed (best-effort: the operator always wants the link back to
// the most recent run that processed the issue).
func TestFinishRun_StampsLastRunOnNativeTracker(t *testing.T) {
	ft := newLastRunTracker()

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "ws")
	storeDir := filepath.Join(dir, "store")
	cfg := &Config{
		Name:      "test",
		Workflow:  t.TempDir() + "/fake.iter",
		Tracker:   TrackerConfig{Kind: "fake"},
		Polling:   PollingConfig{IntervalMS: 50},
		Agent:     AgentConfig{MaxConcurrent: 4, MaxRetryBackoffMS: 1000, RunningState: "in_progress"},
		Workspace: WorkspaceConfig{Root: wsDir},
		Stall:     StallConfig{TimeoutMS: 0},
	}
	cfg.applyDefaults()
	ws, err := NewWorkspaces(wsDir)
	if err != nil {
		t.Fatalf("NewWorkspaces: %v", err)
	}
	c, err := New(Options{
		Config:     cfg,
		Tracker:    ft,
		Runner:     &StubRunner{},
		Workspaces: ws,
		Logger:     iterlog.New(iterlog.LevelError, &bytes.Buffer{}),
		HostMarker: "test",
		StoreDir:   storeDir,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Plant a fake run.json so resolveRunWorkdir picks up the canonical
	// WorkDir (mirrors the worktree:auto case where Run.WorkDir is
	// swapped to the worktree path mid-run).
	runID := "run-stamp-1"
	runDir := filepath.Join(storeDir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	wtPath := filepath.Join(storeDir, "worktrees", runID)
	runJSON, _ := json.Marshal(map[string]any{
		"id":       runID,
		"work_dir": wtPath,
	})
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), runJSON, 0o644); err != nil {
		t.Fatalf("write run.json: %v", err)
	}

	// Seed a running entry directly and invoke finishRun on the actor
	// goroutine via the cmd channel — this exercises the same path
	// cmdRunFinished.apply uses, without spinning up the full polling
	// loop. ctx parent for the entry is background; revertTransition's
	// internal release uses its own timeout-bound ctx.
	issueID := "fake:stamp-1"
	ft.add(tracker.Issue{
		ID: issueID, Identifier: "fake#stamp-1",
		Title: "go", WorkflowState: "in_progress",
	})

	c.state.running[issueID] = &runningEntry{
		IssueID:       issueID,
		Identifier:    "fake#stamp-1",
		RunID:         runID,
		WorkflowState: "in_progress",
		WorkspacePath: filepath.Join(wsDir, "fake_stamp-1"),
		StartedAt:     time.Now(),
	}

	c.finishRun(context.Background(), issueID, nil)

	calls := ft.lastRunCalls()
	if len(calls) != 1 {
		t.Fatalf("SetLastRun calls = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.id != issueID {
		t.Fatalf("call id = %q, want %q", got.id, issueID)
	}
	if got.runID != runID {
		t.Fatalf("call runID = %q, want %q", got.runID, runID)
	}
	// The dispatcher must prefer the run.json work_dir over the
	// dispatcher's WorkspacePath — that's the load-bearing piece for
	// worktree:auto runs whose diff lives outside the workspace dir.
	if got.workdir != wtPath {
		t.Fatalf("call workdir = %q, want %q (from run.json)", got.workdir, wtPath)
	}
}

// TestFinishRun_StampsOnFailureToo asserts the stamp lands even when
// the run failed — the operator still wants to inspect the partial
// diff via the worktree link.
func TestFinishRun_StampsOnFailureToo(t *testing.T) {
	ft := newLastRunTracker()

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "ws")
	cfg := &Config{
		Name:      "test",
		Workflow:  t.TempDir() + "/fake.iter",
		Tracker:   TrackerConfig{Kind: "fake"},
		Polling:   PollingConfig{IntervalMS: 50},
		Agent:     AgentConfig{MaxConcurrent: 4, MaxRetryBackoffMS: 1000, RunningState: "in_progress"},
		Workspace: WorkspaceConfig{Root: wsDir},
		Stall:     StallConfig{TimeoutMS: 0},
	}
	cfg.applyDefaults()
	ws, err := NewWorkspaces(wsDir)
	if err != nil {
		t.Fatalf("NewWorkspaces: %v", err)
	}
	c, err := New(Options{
		Config:     cfg,
		Tracker:    ft,
		Runner:     &StubRunner{},
		Workspaces: ws,
		Logger:     iterlog.New(iterlog.LevelError, &bytes.Buffer{}),
		HostMarker: "test",
		// No StoreDir → resolveRunWorkdir returns "" → falls back to
		// WorkspacePath. Asserts the fallback path is wired.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	issueID := "fake:stamp-2"
	wsPath := filepath.Join(wsDir, "fake_stamp-2")
	ft.add(tracker.Issue{
		ID: issueID, Identifier: "fake#stamp-2",
		Title: "go", WorkflowState: "in_progress",
	})
	c.state.running[issueID] = &runningEntry{
		IssueID:               issueID,
		Identifier:            "fake#stamp-2",
		RunID:                 "run-fail-1",
		WorkflowState:         "in_progress",
		WorkspacePath:         wsPath,
		StartedAt:             time.Now(),
		TransitionedFromState: "ready",
	}

	// Use a non-cancellation error so we hit the failure (not soft-stop)
	// branch — both branches stamp, but explicitly exercising the
	// non-cancel path guards against a regression where stampLastRun
	// gets accidentally tucked under the err==nil arm.
	c.finishRun(context.Background(), issueID, errPermanentFailure{})

	calls := ft.lastRunCalls()
	if len(calls) != 1 {
		t.Fatalf("SetLastRun calls = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.workdir != wsPath {
		t.Fatalf("fallback workdir = %q, want WorkspacePath %q", got.workdir, wsPath)
	}
	if got.runID != "run-fail-1" {
		t.Fatalf("runID = %q, want run-fail-1", got.runID)
	}
}

// errPermanentFailure is a tagged error that fakeTracker doesn't
// recognise as cancellation — used to drive the failure branch of
// finishRun above.
type errPermanentFailure struct{}

func (errPermanentFailure) Error() string { return "permanent failure" }

// TestDispatch_StampsLastRunAtStart asserts the studio "run ↗" link is
// live for the whole run: the dispatcher must stamp last_run when it
// dispatches, not only when the run finishes. A blocking runner keeps
// the run in flight so the only SetLastRun observed is the dispatch-time
// one (the finish-time stamp can't have fired yet).
func TestDispatch_StampsLastRunAtStart(t *testing.T) {
	ft := newLastRunTracker()
	ft.add(tracker.Issue{
		ID: "fake:dispatch-stamp", Identifier: "fake#dispatch-stamp",
		Title: "go", WorkflowState: "ready",
	})

	started := make(chan struct{}, 1)
	runner := &StubRunner{Handler: func(ctx context.Context, _ DispatchSpec) error {
		started <- struct{}{}
		<-ctx.Done() // keep the run "in flight" until the test cancels
		return ctx.Err()
	}}

	dir := t.TempDir()
	cfg := &Config{
		Name:      "test",
		Workflow:  t.TempDir() + "/fake.iter",
		Tracker:   TrackerConfig{Kind: "fake"},
		Polling:   PollingConfig{IntervalMS: 30},
		Agent:     AgentConfig{MaxConcurrent: 4, MaxRetryBackoffMS: 1000, RunningState: "in_progress"},
		Workspace: WorkspaceConfig{Root: filepath.Join(dir, "ws")},
		Stall:     StallConfig{TimeoutMS: 0},
	}
	cfg.applyDefaults()
	ws, err := NewWorkspaces(filepath.Join(dir, "ws"))
	if err != nil {
		t.Fatalf("NewWorkspaces: %v", err)
	}
	c, err := New(Options{
		Config:     cfg,
		Tracker:    ft,
		Runner:     runner,
		Workspaces: ws,
		Logger:     iterlog.New(iterlog.LevelError, &bytes.Buffer{}),
		HostMarker: "test",
		StoreDir:   filepath.Join(dir, "store"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	defer c.Stop()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch never started")
	}

	// The run is now blocked in the handler — finishRun cannot have run,
	// so any SetLastRun call must be the dispatch-time stamp.
	calls := ft.lastRunCalls()
	if len(calls) == 0 {
		t.Fatal("expected last_run stamped at dispatch, got none")
	}
	got := calls[0]
	if got.id != "fake:dispatch-stamp" {
		t.Fatalf("stamped issue = %q, want fake:dispatch-stamp", got.id)
	}
	if got.runID == "" {
		t.Fatalf("dispatch-time stamp must carry a runID, got empty")
	}
}

// TestFinishRun_WarnsOnZeroCommit asserts the honesty guard: a clean
// finish whose run produced no commit is logged at WARN (not the
// silent INFO "finished cleanly") so an empty run doesn't masquerade
// as completed work — while still transitioning the issue to
// CompletedState (reverting would loop the dispatcher). A run that
// produced a commit keeps the quiet INFO path.
func TestFinishRun_WarnsOnZeroCommit(t *testing.T) {
	run := func(t *testing.T, finalCommit string) (logs string, finalState string) {
		ft := newFakeTracker()
		dir := t.TempDir()
		wsDir := filepath.Join(dir, "ws")
		storeDir := filepath.Join(dir, "store")
		cfg := &Config{
			Name:      "test",
			Workflow:  t.TempDir() + "/fake.iter",
			Tracker:   TrackerConfig{Kind: "fake"},
			Polling:   PollingConfig{IntervalMS: 50},
			Agent:     AgentConfig{MaxConcurrent: 4, MaxRetryBackoffMS: 1000, RunningState: "in_progress"},
			Workspace: WorkspaceConfig{Root: wsDir},
			Stall:     StallConfig{TimeoutMS: 0},
		}
		cfg.applyDefaults() // CompletedState defaults to "review"
		ws, err := NewWorkspaces(wsDir)
		if err != nil {
			t.Fatalf("NewWorkspaces: %v", err)
		}
		var buf bytes.Buffer
		c, err := New(Options{
			Config:     cfg,
			Tracker:    ft,
			Runner:     &StubRunner{},
			Workspaces: ws,
			Logger:     iterlog.New(iterlog.LevelInfo, &buf),
			HostMarker: "test",
			StoreDir:   storeDir,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		runID := "run-empty-check"
		runDir := filepath.Join(storeDir, "runs", runID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("mkdir run dir: %v", err)
		}
		payload := map[string]any{"id": runID, "work_dir": filepath.Join(storeDir, "worktrees", runID)}
		if finalCommit != "" {
			payload["final_commit"] = finalCommit
		}
		runJSON, _ := json.Marshal(payload)
		if err := os.WriteFile(filepath.Join(runDir, "run.json"), runJSON, 0o644); err != nil {
			t.Fatalf("write run.json: %v", err)
		}

		issueID := "fake:empty-1"
		ft.add(tracker.Issue{
			ID: issueID, Identifier: "fake#empty-1",
			Title: "go", WorkflowState: "in_progress",
		})
		c.state.running[issueID] = &runningEntry{
			IssueID:       issueID,
			Identifier:    "fake#empty-1",
			RunID:         runID,
			WorkflowState: "in_progress",
			WorkspacePath: filepath.Join(wsDir, "fake_empty-1"),
			StartedAt:     time.Now(),
		}

		c.finishRun(context.Background(), issueID, nil)

		states, _ := ft.RefreshStates(context.Background(), []string{issueID})
		return buf.String(), states[issueID]
	}

	t.Run("zero commit warns", func(t *testing.T) {
		logs, state := run(t, "")
		if !strings.Contains(logs, "produced NO commit") {
			t.Fatalf("expected zero-commit WARN, got logs:\n%s", logs)
		}
		// Non-breaking: the issue still advances to CompletedState.
		if state != "review" {
			t.Fatalf("issue state = %q, want %q (transition must still happen)", state, "review")
		}
	})

	t.Run("with commit stays quiet", func(t *testing.T) {
		logs, state := run(t, "deadbeefcafe")
		if strings.Contains(logs, "produced NO commit") {
			t.Fatalf("did not expect zero-commit WARN for a committed run, got logs:\n%s", logs)
		}
		if !strings.Contains(logs, "finished cleanly") {
			t.Fatalf("expected quiet INFO 'finished cleanly', got logs:\n%s", logs)
		}
		if state != "review" {
			t.Fatalf("issue state = %q, want %q", state, "review")
		}
	})
}
