package dispatcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
)

// TransitionMergedIssue covers the studio's "auto-move ticket on merge"
// hook. The cases below pin the four early-exit paths (nil dispatcher,
// empty issue id, empty MergedState, "none" sentinel) AND the happy
// path that drives the tracker. Operators on a board with no MergedState
// configured rely on the silent no-op — without this guard, every
// studio-driven merge would 4xx-noise the logs.

func TestManager_TransitionMergedIssue_NoDispatcher(t *testing.T) {
	dir := newTestStoreDir(t)
	m, err := NewManager(ManagerOptions{
		StoreDir:    dir,
		NativeStore: newTestNativeStore(t, dir),
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Manager is idle (no Start). Cur is nil — the call must no-op silently.
	if err := m.TransitionMergedIssue(context.Background(), "native:abc"); err != nil {
		t.Errorf("idle manager: TransitionMergedIssue err = %v, want nil", err)
	}
}

func TestManager_TransitionMergedIssue_EmptyIssue(t *testing.T) {
	t.Setenv("ITERION_DISPATCHER_AUTOSTART", "0")
	dir := seedManagerFixture(t)
	m, err := NewManager(ManagerOptions{
		StoreDir:    dir,
		NativeStore: newTestNativeStore(t, dir),
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Stop()
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// No issue id → silent no-op even when MergedState is configured.
	if err := m.TransitionMergedIssue(context.Background(), ""); err != nil {
		t.Errorf("empty issue: TransitionMergedIssue err = %v, want nil", err)
	}
}

func TestManager_TransitionMergedIssue_EmptyAndNoneAreNoOp(t *testing.T) {
	// Two distinct ways an operator says "don't auto-transition":
	// MergedState left empty (default) OR set to the explicit "none"
	// sentinel. Both must short-circuit before the tracker is touched.
	for _, mergedState := range []string{"", "none"} {
		t.Run("merged_state="+mergedState, func(t *testing.T) {
			t.Setenv("ITERION_DISPATCHER_AUTOSTART", "0")
			dir := seedManagerFixtureWithMergedState(t, mergedState)
			ns := newTestNativeStore(t, dir)
			iss, err := ns.Create(native.Issue{Title: "demo"})
			if err != nil {
				t.Fatalf("create issue: %v", err)
			}
			m, err := NewManager(ManagerOptions{
				StoreDir:    dir,
				NativeStore: ns,
				Logger:      newTestLogger(),
			})
			if err != nil {
				t.Fatalf("NewManager: %v", err)
			}
			defer m.Stop()
			if err := m.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			before := iss.State
			if err := m.TransitionMergedIssue(context.Background(), iss.ID); err != nil {
				t.Errorf("merged_state=%q: TransitionMergedIssue err = %v", mergedState, err)
			}
			got, err := ns.Get(iss.ID)
			if err != nil {
				t.Fatalf("reload issue: %v", err)
			}
			if got.State != before {
				t.Errorf("merged_state=%q: state changed %q → %q, expected unchanged", mergedState, before, got.State)
			}
		})
	}
}

func TestManager_TransitionMergedIssue_HappyPath(t *testing.T) {
	// Operator configures merged_state=done; the studio's merge hook
	// fires for a known issue → tracker.UpdateState lands → next board
	// read shows the issue in the target state. End-to-end happy path.
	t.Setenv("ITERION_DISPATCHER_AUTOSTART", "0")
	dir := seedManagerFixtureWithMergedState(t, native.StateDone)
	ns := newTestNativeStore(t, dir)
	iss, err := ns.Create(native.Issue{Title: "demo", State: native.StateReview})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	m, err := NewManager(ManagerOptions{
		StoreDir:    dir,
		NativeStore: ns,
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Stop()
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.TransitionMergedIssue(context.Background(), iss.ID); err != nil {
		t.Fatalf("happy path: TransitionMergedIssue err = %v", err)
	}
	got, err := ns.Get(iss.ID)
	if err != nil {
		t.Fatalf("reload issue: %v", err)
	}
	if got.State != native.StateDone {
		t.Errorf("state = %q, want %q", got.State, native.StateDone)
	}
}

// seedManagerFixtureWithMergedState mirrors seedManagerFixture but
// also stamps agent.merged_state into the persisted config so the
// dispatcher Config().Agent.MergedState exposes the value the merge
// hook reads.
func seedManagerFixtureWithMergedState(t *testing.T, mergedState string) string {
	t.Helper()
	dir := newTestStoreDir(t)
	wfPath := filepath.Join(dir, "flow.iter")
	wfBody := `tool noop:
  command: "true"

workflow minimal:
  entry: noop
  noop -> done
`
	if err := os.WriteFile(wfPath, []byte(wfBody), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	cfg := map[string]any{
		"workflow": wfPath,
		"tracker":  map[string]any{"kind": "native"},
		"agent":    map[string]any{"merged_state": mergedState},
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath := filepath.Join(dir, "dispatcher", "dispatcher.json")
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}
