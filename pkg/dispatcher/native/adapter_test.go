package native_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/dispatcher/tracker"
)

func newAdapter(t *testing.T) (*native.Adapter, *native.Store) {
	t.Helper()
	s, err := native.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return native.NewAdapter(s), s
}

func TestListCandidatesEligibleOnly(t *testing.T) {
	a, s := newAdapter(t)

	ready, _ := s.Create(native.Issue{Title: "go", State: "ready"})
	_, _ = s.Create(native.Issue{Title: "later", State: "backlog"})
	claimed, _ := s.Create(native.Issue{Title: "taken", State: "ready"})
	if err := s.Claim(claimed.ID, "other"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	got, err := a.ListCandidates(context.Background())
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(got) != 1 || got[0].ID != ready.ID {
		t.Fatalf("want only ready, got %+v", got)
	}
	if got[0].WorkflowState != "ready" {
		t.Fatalf("workflow state not propagated: %s", got[0].WorkflowState)
	}
}

func TestListCandidatesBlockerGating(t *testing.T) {
	a, s := newAdapter(t)

	blocker, _ := s.Create(native.Issue{Title: "blocker", State: "in_progress"})
	gated, _ := s.Create(native.Issue{Title: "needs blocker", State: "ready", Blockers: []string{blocker.ID}})

	candidates, _ := a.ListCandidates(context.Background())
	for _, c := range candidates {
		if c.ID == gated.ID {
			t.Fatalf("blocked issue should not be a candidate")
		}
	}

	if _, err := s.SetState(blocker.ID, "done"); err != nil {
		t.Fatalf("close blocker: %v", err)
	}
	candidates, _ = a.ListCandidates(context.Background())
	found := false
	for _, c := range candidates {
		if c.ID == gated.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("gated issue should be a candidate after blocker closed")
	}
}

func TestRefreshStates(t *testing.T) {
	a, s := newAdapter(t)
	x, _ := s.Create(native.Issue{Title: "x", State: "ready"})
	y, _ := s.Create(native.Issue{Title: "y", State: "in_progress"})

	got, err := a.RefreshStates(context.Background(), []string{x.ID, y.ID, "native:missing"})
	if err != nil {
		t.Fatalf("RefreshStates: %v", err)
	}
	if got[x.ID] != "ready" || got[y.ID] != "in_progress" {
		t.Fatalf("bad states: %v", got)
	}
	if _, ok := got["native:missing"]; ok {
		t.Fatal("missing ID should be omitted")
	}
}

func TestAdapterClaimRelease(t *testing.T) {
	a, s := newAdapter(t)
	iss, _ := s.Create(native.Issue{Title: "x", State: "ready"})

	if err := a.Claim(context.Background(), iss.ID, "host-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := a.Claim(context.Background(), iss.ID, "host-2"); !errors.Is(err, tracker.ErrClaimConflict) {
		t.Fatalf("want ErrClaimConflict, got %v", err)
	}
	if err := a.Release(context.Background(), iss.ID, "host-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := a.Claim(context.Background(), iss.ID, "host-2"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
}

func TestCommentNotSupported(t *testing.T) {
	a, _ := newAdapter(t)
	err := a.Comment(context.Background(), "native:x", "hi")
	if !errors.Is(err, tracker.ErrNotSupported) {
		t.Fatalf("want ErrNotSupported, got %v", err)
	}
}

func TestAdapterUpdateState(t *testing.T) {
	a, s := newAdapter(t)
	iss, _ := s.Create(native.Issue{Title: "x", State: "ready"})
	if err := a.UpdateState(context.Background(), iss.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, _ := s.Get(iss.ID)
	if got.State != "in_progress" {
		t.Fatalf("state not updated: %s", got.State)
	}
}

// Compile-time assertion that *Adapter satisfies tracker.Tracker.
var _ tracker.Tracker = (*native.Adapter)(nil)
