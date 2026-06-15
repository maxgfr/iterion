package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// A torn line (crash/OOM/ENOSPC mid-append) must not brick the inbox:
// the valid records still load. Before the fix, loadLatestQueuedMessages
// returned an error on the first bad line, so ListQueuedMessages /
// LoadPendingQueuedMessages / UpdateQueuedMessageStatus all 500'd for
// the rest of the run's life.
func TestLoadQueuedMessagesToleratesTornLine(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	const runID = "run-inbox"

	if err := s.AppendQueuedMessage(ctx, runID, QueuedUserMessage{
		ID:       "m1",
		Text:     "hello",
		QueuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendQueuedMessage: %v", err)
	}

	// Simulate a crash mid-append: a partial JSON record with no
	// trailing newline at EOF.
	path, err := s.userMessagesPath(runID)
	if err != nil {
		t.Fatalf("userMessagesPath: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString(`{"id":"m2","run_id":"run-inbox","sta`); err != nil {
		t.Fatalf("write torn line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	msgs, err := s.ListQueuedMessages(ctx, runID)
	if err != nil {
		t.Fatalf("ListQueuedMessages after torn write: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "m1" {
		t.Fatalf("got %d messages %+v; want exactly m1", len(msgs), msgs)
	}

	pending, err := s.LoadPendingQueuedMessages(ctx, runID)
	if err != nil {
		t.Fatalf("LoadPendingQueuedMessages after torn write: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "m1" {
		t.Fatalf("got %d pending %+v; want exactly m1", len(pending), pending)
	}
}

// Wholesale corruption (not a single torn tail) must still fail loudly,
// matching the events.jsonl policy — a single skip is benign, mass
// garbage is not.
func TestLoadQueuedMessagesFailsLoudOnMassCorruption(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	const runID = "run-garbage"

	if err := s.AppendQueuedMessage(ctx, runID, QueuedUserMessage{
		ID:       "m1",
		Text:     "ok",
		QueuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendQueuedMessage: %v", err)
	}
	path, err := s.userMessagesPath(runID)
	if err != nil {
		t.Fatalf("userMessagesPath: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := f.WriteString("this is not json\n"); err != nil {
			t.Fatalf("write garbage: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := s.ListQueuedMessages(ctx, runID); !errors.Is(err, ErrEventsCorrupted) {
		t.Fatalf("ListQueuedMessages on mass corruption = %v; want ErrEventsCorrupted", err)
	}
}
