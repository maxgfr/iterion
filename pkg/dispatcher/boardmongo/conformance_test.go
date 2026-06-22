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
}
