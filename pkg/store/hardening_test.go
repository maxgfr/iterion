package store

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// Regression tests locking in the pkg/store production-readiness hardening
// surfaced by a whole-improve-loop (Willy) cross-family review of the
// run-persistence layer (2026-06-14, run 019ec7ed). Each test corresponds to
// a reviewer blocker; see docs/bot-runs/whole-improve-loop.md:
//
//	B1 — TeeRunLog must reject an unsafe run ID before touching the FS and
//	     create the run dir / run.log with the store's private perms (run
//	     logs hold prompts, model output, and secrets).
//	B2 — AppendEvent must sanitize the run ID (traversal-defense parity with
//	     LoadRun / LoadEvents / Artifact / Interaction, which already did).
//	B3 — CreateRun must be exclusive (no-clobber) so a re-used run ID cannot
//	     reset an existing run's metadata / checkpoint.
//	B4 — AppendEvent must repair a torn final JSONL line left by a prior
//	     crash, so the first post-resume event is not lost to concatenation.

// B3: CreateRun is a no-clobber exclusive create.
func TestCreateRunIsExclusive(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()

	if _, err := s.CreateRun(ctx, "dup", "wf", map[string]interface{}{"k": "v1"}); err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}

	_, err := s.CreateRun(ctx, "dup", "wf", map[string]interface{}{"k": "v2"})
	if err == nil {
		t.Fatal("second CreateRun with a re-used ID: expected error, got nil (run was clobbered)")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second CreateRun: error = %v, want fs.ErrExist in the chain", err)
	}

	// The original run's metadata must survive the rejected re-create.
	r, err := s.LoadRun(ctx, "dup")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if got, _ := r.Inputs["k"].(string); got != "v1" {
		t.Fatalf("inputs[k] = %q, want v1 (original was clobbered by the second create)", got)
	}
}

// B2: AppendEvent rejects an unsafe run ID (the traversal-defense gap that
// LoadRun / LoadEvents / Artifact / Interaction already closed).
func TestAppendEventRejectsUnsafeRunID(t *testing.T) {
	s := tmpStore(t)
	if _, err := s.AppendEvent(context.Background(), "../../escape", Event{Type: EventRunStarted}); err == nil {
		t.Fatal("AppendEvent with a traversal run ID: expected error, got nil")
	}
}

// B4: a torn final JSONL line (a partial write from a prior crash) is
// separated from the next event so the first post-resume event survives the
// replay instead of being concatenated into a single corrupt line.
func TestAppendEventRepairsTornTail(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()

	if _, err := s.CreateRun(ctx, "torn", "wf", nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := s.AppendEvent(ctx, "torn", Event{Type: EventRunStarted}); err != nil {
		t.Fatalf("append event 1: %v", err)
	}

	// Simulate a crash mid-write: a partial JSONL record with no trailing
	// newline left at the end of events.jsonl.
	f, err := os.OpenFile(s.eventsPath("torn"), os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		t.Fatalf("open events for torn write: %v", err)
	}
	if _, err := f.WriteString(`{"seq":99,"type":"node_star`); err != nil {
		t.Fatalf("write torn tail: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A fresh store (a new process, e.g. on resume) appends the next event.
	s2, err := New(s.Root())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s2.AppendEvent(ctx, "torn", Event{Type: EventNodeStarted}); err != nil {
		t.Fatalf("append after torn tail: %v", err)
	}

	evts, err := s2.LoadEvents(ctx, "torn")
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	// The torn partial line is skipped, but BOTH valid events survive. Without
	// the repair, event 2 concatenates onto the torn bytes and both vanish
	// (len would be 1).
	if len(evts) != 2 {
		var got []string
		for _, e := range evts {
			got = append(got, string(e.Type))
		}
		t.Fatalf("LoadEvents returned %d events %v, want 2 (torn line skipped, both valid events kept)", len(evts), got)
	}
	if evts[0].Type != EventRunStarted || evts[1].Type != EventNodeStarted {
		t.Fatalf("events = [%s, %s], want [run_started, node_started]", evts[0].Type, evts[1].Type)
	}
	if evts[0].Seq != 0 || evts[1].Seq != 1 {
		t.Fatalf("seqs = [%d, %d], want [0, 1] (monotonic across the torn tail)", evts[0].Seq, evts[1].Seq)
	}
}

// B1: TeeRunLog refuses an unsafe run ID before touching the FS, and creates
// the run dir + run.log with the store's private perms.
func TestTeeRunLogHardening(t *testing.T) {
	root := t.TempDir()
	logger := iterlog.New(iterlog.LevelError, io.Discard)

	// Unsafe run ID: no tee, and no filesystem is touched under the store.
	if _, closer := TeeRunLog(logger, iterlog.LevelError, root, "../escape"); closer != nil {
		_ = closer.Close()
		t.Fatal("TeeRunLog with an unsafe run ID returned a non-nil closer")
	}
	if _, err := os.Stat(filepath.Join(root, "runs")); !os.IsNotExist(err) {
		t.Fatalf("TeeRunLog with an unsafe run ID created %s/runs (stat err = %v)", root, err)
	}

	// Safe run ID: tee set up with private perms on both the dir and the file.
	_, closer := TeeRunLog(logger, iterlog.LevelError, root, "safe")
	if closer == nil {
		t.Fatal("TeeRunLog with a safe run ID returned a nil closer")
	}
	_ = closer.Close()

	runDir := filepath.Join(root, "runs", "safe")
	if di, err := os.Stat(runDir); err != nil {
		t.Fatalf("stat run dir: %v", err)
	} else if di.Mode().Perm() != dirPerm {
		t.Errorf("run dir perm = %#o, want %#o", di.Mode().Perm(), dirPerm)
	}
	if fi, err := os.Stat(filepath.Join(runDir, "run.log")); err != nil {
		t.Fatalf("stat run.log: %v", err)
	} else if fi.Mode().Perm() != filePerm {
		t.Errorf("run.log perm = %#o, want %#o", fi.Mode().Perm(), filePerm)
	}
}
