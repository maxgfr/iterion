// Package queue is a small FIFO job queue backed by a buffered channel.
package queue

import "errors"

// Job is the unit of work a worker processes.
type Job struct {
	ID      string
	Payload []byte
}

// ErrFull is returned by Enqueue when the channel is at capacity.
var ErrFull = errors.New("queue full")

// Queue holds pending jobs. A nil-zero queue is unusable — call New.
type Queue struct {
	ch chan Job
}

// New returns a Queue with the given buffer size.
func New(buffer int) *Queue {
	return &Queue{ch: make(chan Job, buffer)}
}

// Enqueue pushes j onto the queue. Returns ErrFull when the channel
// is full so callers can decide whether to drop or retry.
func (q *Queue) Enqueue(j Job) error {
	select {
	case q.ch <- j:
		return nil
	default:
		return ErrFull
	}
}

// Dequeue blocks until a job arrives. Returns the job and true, or
// the zero job and false if the channel was closed.
func (q *Queue) Dequeue() (Job, bool) {
	j := <-q.ch
	return j, true
}

// Close marks the queue as drained. Callers must guarantee no more
// Enqueue calls land — a send on a closed channel panics.
func (q *Queue) Close() {
	close(q.ch)
}
