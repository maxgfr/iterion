package queue

import "testing"

func TestEnqueueDequeue(t *testing.T) {
	q := New(2)
	if err := q.Enqueue(Job{ID: "a"}); err != nil {
		t.Fatalf("enqueue a: %v", err)
	}
	if err := q.Enqueue(Job{ID: "b"}); err != nil {
		t.Fatalf("enqueue b: %v", err)
	}
	got, _ := q.Dequeue()
	if got.ID != "a" {
		t.Errorf("dequeue 1: got %q, want a", got.ID)
	}
}

func TestEnqueueFull(t *testing.T) {
	q := New(1)
	_ = q.Enqueue(Job{ID: "a"})
	if err := q.Enqueue(Job{ID: "b"}); err != ErrFull {
		t.Errorf("enqueue when full: got %v, want ErrFull", err)
	}
}
