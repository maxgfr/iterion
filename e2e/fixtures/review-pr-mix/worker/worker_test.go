package worker

import (
	"sync"
	"testing"
	"time"

	"example.com/jobqueue/queue"
)

func TestWorkerID(t *testing.T) {
	q := queue.New(1)
	w := New(q, "w1")
	if w.ID() != "w1" {
		t.Errorf("ID = %q, want w1", w.ID())
	}
}

// TestWorkerProcessesJob seeds one job and waits for fn to fire.
// The worker goroutine is intentionally not stopped here — the test
// process exits on its own.
func TestWorkerProcessesJob(t *testing.T) {
	q := queue.New(1)
	w := New(q, "w-test")
	var (
		mu   sync.Mutex
		seen []string
	)
	go w.Run(func(j queue.Job) error {
		mu.Lock()
		seen = append(seen, j.ID)
		mu.Unlock()
		return nil
	})
	if err := q.Enqueue(queue.Job{ID: "j1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := append([]string{}, seen...)
	mu.Unlock()
	if len(got) != 1 || got[0] != "j1" {
		t.Errorf("seen = %v, want [j1]", got)
	}
}
