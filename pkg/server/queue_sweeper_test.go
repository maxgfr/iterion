package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/store"
	mongostore "github.com/SocialGouv/iterion/pkg/store/mongo"
)

type fakeStaleLister struct {
	refs map[string][]mongostore.StaleRunRef // status -> refs
}

func (f *fakeStaleLister) ListStaleActiveRuns(_ context.Context, statuses []store.RunStatus, _ time.Time, _ int) ([]mongostore.StaleRunRef, error) {
	var out []mongostore.StaleRunRef
	for _, st := range statuses {
		out = append(out, f.refs[string(st)]...)
	}
	return out, nil
}

type fakeLeases struct{ locked map[string]bool }

func (f *fakeLeases) IsRunLocked(_ context.Context, runID string) (bool, error) {
	return f.locked[runID], nil
}

type fakeSweepStore struct {
	store.RunStore
	mu      sync.Mutex
	flipped map[string]store.RunStatus
}

func (f *fakeSweepStore) UpdateRunStatusIf(ctx context.Context, id string, status store.RunStatus, _ string, _ []store.RunStatus) (bool, error) {
	// The sweeper must stamp the run's tenant before the CAS — assert
	// the ctx carries one (the mongo store would panic otherwise).
	if tenant, ok := store.TenantFromContext(ctx); !ok || tenant == "" {
		panic("sweeper CAS without tenant ctx")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.flipped == nil {
		f.flipped = map[string]store.RunStatus{}
	}
	f.flipped[id] = status
	return true, nil
}

func TestSweepOrphanRuns(t *testing.T) {
	s := newOrgTestServer(t)
	fs := &fakeSweepStore{}
	s.cfg.Store = fs
	lister := &fakeStaleLister{refs: map[string][]mongostore.StaleRunRef{
		"queued":  {{ID: "r-queued", TenantID: "t1", Status: "queued"}},
		"running": {{ID: "r-crashed", TenantID: "t2", Status: "running"}, {ID: "r-healthy", TenantID: "t3", Status: "running"}},
	}}
	leases := &fakeLeases{locked: map[string]bool{"r-healthy": true}}

	s.sweepOrphanRuns(context.Background(), lister, leases, time.Now().UTC())

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.flipped["r-queued"] != store.RunStatusFailedResumable {
		t.Fatalf("queued orphan not flipped: %+v", fs.flipped)
	}
	if fs.flipped["r-crashed"] != store.RunStatusFailedResumable {
		t.Fatalf("crashed running orphan not flipped: %+v", fs.flipped)
	}
	if _, ok := fs.flipped["r-healthy"]; ok {
		t.Fatal("leased (in-flight) run was flipped — the lease check must protect it")
	}
}
