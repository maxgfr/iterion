package server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dispatcher/boardmongo"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
)

type fakeBoardCoord struct {
	mu       sync.Mutex
	cands    []boardmongo.Candidate
	claimed  map[string]string
	states   map[string]string
	claimErr map[string]error
}

func newFakeBoardCoord(cands ...boardmongo.Candidate) *fakeBoardCoord {
	return &fakeBoardCoord{cands: cands, claimed: map[string]string{}, states: map[string]string{}, claimErr: map[string]error{}}
}

func (f *fakeBoardCoord) ListEligible(_ context.Context, _ []string, _ int) ([]boardmongo.Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []boardmongo.Candidate
	for _, c := range f.cands {
		if f.claimed[c.Issue.ID] == "" {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeBoardCoord) Claim(_ context.Context, _, id, marker string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.claimErr[id]; err != nil {
		return err
	}
	if f.claimed[id] != "" {
		return errors.New("conflict")
	}
	f.claimed[id] = marker
	return nil
}

func (f *fakeBoardCoord) SetState(_ context.Context, _, id, state string) error {
	f.mu.Lock()
	f.states[id] = state
	f.mu.Unlock()
	return nil
}

func (f *fakeBoardCoord) Release(_ context.Context, _, id, _ string) error {
	f.mu.Lock()
	delete(f.claimed, id)
	f.mu.Unlock()
	return nil
}

func readyCard(id, bot string) boardmongo.Candidate {
	return boardmongo.Candidate{Tenant: "t1", Issue: native.Issue{ID: id, Bot: bot, State: native.StateReady}}
}

func TestBoardDispatcher_ClaimsProcessesAndMoves(t *testing.T) {
	f := newFakeBoardCoord(readyCard("native:1", "feature-dev"), readyCard("native:2", "sec-audit-source"))
	var pmu sync.Mutex
	processed := map[string]int{}
	d := newBoardDispatcher(f, func(_ context.Context, _ string, iss native.Issue) error {
		pmu.Lock()
		processed[iss.ID]++
		pmu.Unlock()
		return nil
	}, "replica-A", 4, nil)

	if n := d.tick(context.Background()); n != 2 {
		t.Fatalf("tick should claim 2, got %d", n)
	}
	d.wg.Wait()

	if processed["native:1"] != 1 || processed["native:2"] != 1 {
		t.Errorf("each card should process once: %v", processed)
	}
	if f.states["native:1"] != native.StateDone || f.states["native:2"] != native.StateDone {
		t.Errorf("cards should move to done: %v", f.states)
	}
	if len(f.claimed) != 0 {
		t.Errorf("cards should be released: %v", f.claimed)
	}
}

func TestBoardDispatcher_FailedRunMovesToBlocked(t *testing.T) {
	f := newFakeBoardCoord(readyCard("native:1", "feature-dev"))
	d := newBoardDispatcher(f, func(context.Context, string, native.Issue) error {
		return errors.New("run failed")
	}, "replica-A", 4, nil)
	d.tick(context.Background())
	d.wg.Wait()
	if f.states["native:1"] != native.StateBlocked {
		t.Errorf("failed run should move card to blocked, got %q", f.states["native:1"])
	}
}

func TestBoardDispatcher_ClaimConflictSkips(t *testing.T) {
	f := newFakeBoardCoord(readyCard("native:1", "feature-dev"))
	f.claimErr["native:1"] = errors.New("held by another replica")
	var processed int
	d := newBoardDispatcher(f, func(context.Context, string, native.Issue) error {
		processed++
		return nil
	}, "replica-A", 4, nil)
	if n := d.tick(context.Background()); n != 0 {
		t.Fatalf("claim conflict should yield 0 claims, got %d", n)
	}
	d.wg.Wait()
	if processed != 0 {
		t.Errorf("a card we couldn't claim must not be processed, got %d", processed)
	}
	// The semaphore slot must have been released on the failed claim.
	if len(d.sem) != 0 {
		t.Errorf("semaphore slot leaked on failed claim: %d held", len(d.sem))
	}
}
