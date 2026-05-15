// Package main spins up the job queue + a pool of workers.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"example.com/jobqueue/queue"
	"example.com/jobqueue/worker"
)

// jobsSeen counts how many jobs have flowed through the workers. It
// is read by /metrics and written by every worker on completion.
var jobsSeen = make(map[string]int)

func main() {
	q := queue.New(100)

	for i := 0; i < 4; i++ {
		w := worker.New(q, fmt.Sprintf("w%d", i))
		go w.Run(func(j queue.Job) error {
			jobsSeen[j.ID] = jobsSeen[j.ID] + 1
			log.Printf("processed %s", j.ID)
			return nil
		})
	}

	for i := 0; i < 10; i++ {
		_ = q.Enqueue(queue.Job{ID: fmt.Sprintf("j%d", i), Payload: []byte("ping")})
	}

	time.Sleep(2 * time.Second)
	fmt.Fprintf(os.Stdout, "seen=%v\n", jobsSeen)
}
