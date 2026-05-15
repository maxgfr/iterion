package conductor

import (
	"math"
	"time"
)

// scheduleRetry queues a retry for the given issue, using exponential
// backoff capped by cfg.MaxRetryBackoff. Must be called from the actor.
func (c *Conductor) scheduleRetry(issueID string, prev *runningEntry, runErr error) {
	cfg := c.cfg.Load()
	prevAttempt := 0
	if cur, ok := c.state.retries[issueID]; ok {
		prevAttempt = cur.Attempt
		if cur.Timer != nil {
			cur.Timer.Stop()
		}
	}
	attempt := prevAttempt + 1
	delay := computeBackoff(attempt, cfg.MaxRetryBackoff())
	due := time.Now().Add(delay)

	timer := time.AfterFunc(delay, func() {
		select {
		case c.cmds <- cmdRetryDue{issueID: issueID}:
		case <-c.stop:
		}
	})
	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}
	c.state.retries[issueID] = &retryEntry{
		IssueID:    issueID,
		Identifier: prev.Identifier,
		Attempt:    attempt,
		DueAt:      due,
		LastError:  errStr,
		Timer:      timer,
	}
	c.logger.Info("conductor: %s retry queued (attempt=%d, in=%s)", prev.Identifier, attempt, delay)
}

// computeBackoff returns min(10s * 2^(attempt-1), cap), with attempt=0
// treated as a continuation (fixed 1s).
func computeBackoff(attempt int, cap time.Duration) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	const base = 10 * time.Second
	// Cap the exponent to avoid int overflow on absurd attempt counts.
	if attempt > 10 {
		attempt = 10
	}
	mult := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(base) * mult)
	if cap > 0 && d > cap {
		return cap
	}
	return d
}
