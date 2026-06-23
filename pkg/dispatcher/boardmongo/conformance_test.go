package boardmongo_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/SocialGouv/iterion/pkg/dispatcher/boardmongo"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/dispatcher/tracker"
)

// runBoardStoreSuite exercises the native.BoardStore contract. It runs against
// both the filesystem native.Store (always — proving the suite) and the Mongo
// store (gated on ITERION_TEST_MONGO_URI), so the two implementations are held
// to an identical bar.
func runBoardStoreSuite(t *testing.T, store native.BoardStore) {
	t.Helper()

	// Create: title required.
	if _, err := store.Create(native.Issue{}); err == nil {
		t.Error("Create without title should fail")
	}

	// Create defaults the state to the board's first state (inbox).
	created, err := store.Create(native.Issue{Title: "first", Labels: []string{"x"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.State != native.StateInbox {
		t.Errorf("default state: want %q, got %q", native.StateInbox, created.State)
	}
	if !strings.HasPrefix(created.ID, "native:") || created.CreatedAt.IsZero() {
		t.Errorf("created issue id/timestamps: %+v", created)
	}

	// Get found + not-found.
	if got, err := store.Get(created.ID); err != nil || got.Title != "first" {
		t.Errorf("Get: %+v err=%v", got, err)
	}
	if _, err := store.Get("native:00000000-0000-0000-0000-000000000000"); !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("Get missing: want ErrNotFound, got %v", err)
	}

	// Resolve by bare uuid (no native: prefix).
	bare := strings.TrimPrefix(created.ID, "native:")
	if full, err := store.Resolve(bare); err != nil || full != created.ID {
		t.Errorf("Resolve(%q): %q err=%v", bare, full, err)
	}

	// Update: patch fields + no-op.
	pr := 5
	updated, err := store.Update(created.ID, native.Patch{Priority: &pr})
	if err != nil || updated.Priority != 5 {
		t.Errorf("Update priority: %+v err=%v", updated, err)
	}
	if _, err := store.Update(created.ID, native.Patch{Priority: &pr}); err != nil {
		t.Errorf("Update no-op: %v", err)
	}

	// set_bot via Update.Bot.
	bot := "feature-dev"
	if u, err := store.Update(created.ID, native.Patch{Bot: &bot}); err != nil || u.Bot != "feature-dev" {
		t.Errorf("Update bot: %+v err=%v", u, err)
	}

	// SetState: valid transition, unknown rejected, no-op same.
	if u, err := store.SetState(created.ID, native.StateReady); err != nil || u.State != native.StateReady {
		t.Errorf("SetState: %+v err=%v", u, err)
	}
	if _, err := store.SetState(created.ID, "does-not-exist"); !errors.Is(err, tracker.ErrTransitionRejected) {
		t.Errorf("SetState unknown: want ErrTransitionRejected, got %v", err)
	}
	if _, err := store.SetState(created.ID, native.StateReady); err != nil {
		t.Errorf("SetState no-op: %v", err)
	}

	// Claim: idempotent same marker; conflict on a different marker; release.
	if err := store.Claim(created.ID, "runner-A"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := store.Claim(created.ID, "runner-A"); err != nil {
		t.Errorf("Claim idempotent: %v", err)
	}
	if err := store.Claim(created.ID, "runner-B"); !errors.Is(err, tracker.ErrClaimConflict) {
		t.Errorf("Claim conflict: want ErrClaimConflict, got %v", err)
	}
	if err := store.Release(created.ID, "runner-B"); !errors.Is(err, tracker.ErrClaimConflict) {
		t.Errorf("Release by wrong marker: want ErrClaimConflict, got %v", err)
	}
	if err := store.Release(created.ID, "runner-A"); err != nil {
		t.Errorf("Release: %v", err)
	}
	if err := store.Release(created.ID, "runner-A"); err != nil {
		t.Errorf("Release unclaimed no-op: %v", err)
	}

	// SetLastRun.
	if err := store.SetLastRun(created.ID, "run-1", "/tmp/wd"); err != nil {
		t.Errorf("SetLastRun: %v", err)
	}
	if got, _ := store.Get(created.ID); got.LastRunID != "run-1" {
		t.Errorf("SetLastRun not persisted: %+v", got)
	}

	// List: filter by state + assignee; sort by priority.
	_, _ = store.Create(native.Issue{Title: "second", State: native.StateReady, Priority: 9})
	ready, err := store.List(native.ListFilter{States: []string{native.StateReady}})
	if err != nil || len(ready) != 2 {
		t.Errorf("List by state: got %d err=%v", len(ready), err)
	}
	if len(ready) == 2 && ready[0].Priority < ready[1].Priority {
		t.Errorf("List should sort by priority desc: %d then %d", ready[0].Priority, ready[1].Priority)
	}

	// AggregateLabels.
	labels := store.AggregateLabels()
	found := false
	for _, l := range labels {
		if l.Label == "x" && l.Count >= 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("AggregateLabels missing label x: %+v", labels)
	}

	// ScanEvents: at least the create/update/state events landed.
	var n int
	if err := store.ScanEvents(func(*native.Event) bool { n++; return true }); err != nil {
		t.Errorf("ScanEvents: %v", err)
	}
	if n == 0 {
		t.Error("ScanEvents returned no events")
	}

	// Delete.
	if err := store.Delete(created.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := store.Get(created.ID); !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("Get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.Delete(created.ID); !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("Delete missing: want ErrNotFound, got %v", err)
	}
}

// TestNativeStore_Conformance proves the suite against the reference
// filesystem implementation (always runs).
func TestNativeStore_Conformance(t *testing.T) {
	store, err := native.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("native.NewStore: %v", err)
	}
	runBoardStoreSuite(t, store)
}

// TestMongoStore_Conformance runs the same suite against the Mongo store.
func TestMongoStore_Conformance(t *testing.T) {
	uri := os.Getenv("ITERION_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("ITERION_TEST_MONGO_URI not set; skipping Mongo board suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	nonce := make([]byte, 4)
	_, _ = rand.Read(nonce)
	db := client.Database("iterion_board_" + hex.EncodeToString(nonce))
	t.Cleanup(func() {
		drop, dc := context.WithTimeout(context.Background(), 10*time.Second)
		defer dc()
		_ = db.Drop(drop)
		_ = client.Disconnect(drop)
	})
	if err := boardmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// Idempotent re-run.
	if err := boardmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema (second): %v", err)
	}
	runBoardStoreSuite(t, boardmongo.New(db, "tenant-1"))

	// The Mongo store must also drive the dispatcher as a tracker.Tracker via
	// the shared native.Adapter (eligible + unclaimed + blocker-free filtering).
	runTrackerSuite(t, boardmongo.New(db, "tracker-tenant"))

	// The Coordinator's cross-tenant ListEligible must find ready+unclaimed
	// cards across tenants (verifies the issue.state / issue.claim BSON paths).
	coord := boardmongo.NewCoordinator(db)
	for _, tc := range []struct {
		tenant, title, state string
		claim                bool
	}{
		{"ca", "ready-a", native.StateReady, false},
		{"cb", "ready-b", native.StateReady, false},
		{"ca", "parked", native.StateInbox, false}, // not eligible
		{"cb", "claimed", native.StateReady, true}, // eligible state but claimed
	} {
		st := coord.StoreFor(tc.tenant)
		iss, cerr := st.Create(native.Issue{Title: tc.title, State: tc.state})
		if cerr != nil {
			t.Fatalf("coord create %s: %v", tc.title, cerr)
		}
		if tc.claim {
			if cerr := st.Claim(iss.ID, "someone"); cerr != nil {
				t.Fatalf("claim: %v", cerr)
			}
		}
	}
	elig, eerr := coord.ListEligible(ctx, []string{native.StateReady}, 50)
	if eerr != nil {
		t.Fatalf("ListEligible: %v", eerr)
	}
	gotTitles := map[string]string{}
	for _, c := range elig {
		gotTitles[c.Issue.Title] = c.Tenant
	}
	if gotTitles["ready-a"] != "ca" || gotTitles["ready-b"] != "cb" {
		t.Errorf("cross-tenant ListEligible should return ready-a + ready-b: %v", gotTitles)
	}
	if _, ok := gotTitles["parked"]; ok {
		t.Error("inbox card must not be eligible")
	}
	if _, ok := gotTitles["claimed"]; ok {
		t.Error("claimed card must not be eligible")
	}
}

// runTrackerSuite exercises the tracker.Tracker view (native.Adapter) over a
// board store — the path the cloud dispatcher uses.
func runTrackerSuite(t *testing.T, store native.BoardStore) {
	t.Helper()
	trk := native.NewAdapter(store)
	ctx := context.Background()

	// An inbox issue is NOT a candidate (inbox is not eligible); a ready issue
	// IS (ready is eligible on the default board).
	_, _ = store.Create(native.Issue{Title: "parked", State: native.StateInbox})
	ready, err := store.Create(native.Issue{Title: "do me", State: native.StateReady})
	if err != nil {
		t.Fatalf("create ready: %v", err)
	}
	cands, err := trk.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != ready.ID {
		t.Fatalf("candidates: want [%s], got %+v", ready.ID, cands)
	}

	// Claim removes it from the candidate set.
	if err := trk.Claim(ctx, ready.ID, "runner-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	cands, _ = trk.ListCandidates(ctx)
	if len(cands) != 0 {
		t.Errorf("claimed issue must not be a candidate, got %+v", cands)
	}

	// UpdateState + RefreshStates round-trip.
	if err := trk.UpdateState(ctx, ready.ID, native.StateDone); err != nil {
		t.Errorf("UpdateState: %v", err)
	}
	states, _ := trk.RefreshStates(ctx, []string{ready.ID})
	if states[ready.ID] != native.StateDone {
		t.Errorf("RefreshStates: %v", states)
	}
}
