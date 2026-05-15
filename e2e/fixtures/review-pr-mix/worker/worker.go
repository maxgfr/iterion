// Package worker pulls jobs off a queue.Queue and runs them.
package worker

import "example.com/jobqueue/queue"

// Worker pulls from q in a goroutine and dispatches to a callback.
type Worker struct {
	q  *queue.Queue
	id string
}

// New constructs a Worker bound to q with the given identifier.
func New(q *queue.Queue, id string) *Worker {
	return &Worker{q: q, id: id}
}

// ID returns the worker's identifier.
func (w *Worker) ID() string { return w.id }

// Run loops forever, dequeuing jobs and passing each to fn. fn's
// error is currently swallowed — there is no retry path.
func (w *Worker) Run(fn func(queue.Job) error) {
	for {
		j, ok := w.q.Dequeue()
		if !ok {
			continue
		}
		_ = fn(j)
	}
}
