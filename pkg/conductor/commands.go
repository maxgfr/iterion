package conductor

import (
	"context"
	"errors"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// cmd is the typed command interface processed by the actor goroutine.
// Each implementation is a small struct so command intent is visible
// in stack traces and log lines.
type cmd interface {
	apply(c *Conductor, ctx context.Context)
}

// cmdRefresh forces an immediate poll tick.
type cmdRefresh struct{}

func (cmdRefresh) apply(c *Conductor, ctx context.Context) { c.tick(ctx) }

// cmdSnapshot returns the current snapshot via reply channel.
type cmdSnapshot struct {
	reply chan Snapshot
}

func (m cmdSnapshot) apply(c *Conductor, _ context.Context) {
	m.reply <- c.buildSnapshot()
}

// cmdReload swaps in a new validated config.
type cmdReload struct {
	cfg *Config
}

func (m cmdReload) apply(c *Conductor, _ context.Context) {
	old := c.cfg.Load()
	c.cfg.Store(m.cfg)
	c.hooks = m.cfg.Hooks
	if old.PollingInterval() != m.cfg.PollingInterval() {
		c.logger.Info("conductor: polling interval %s → %s", old.PollingInterval(), m.cfg.PollingInterval())
	}
	c.fireSnapshot()
}

// cmdEvent updates the last-event watermark on a running issue.
type cmdEvent struct {
	issueID   string
	eventName string
}

func (m cmdEvent) apply(c *Conductor, _ context.Context) {
	r, ok := c.state.running[m.issueID]
	if !ok {
		return
	}
	r.LastEventAt = time.Now().UTC()
	r.LastEventName = m.eventName
}

// cmdRunFinished is posted by the dispatch goroutine when the run
// returns from Runner.Dispatch. err is non-nil on failure (including
// cancellation). The actor decides between continuation, retry, or
// release.
type cmdRunFinished struct {
	issueID string
	err     error
}

func (m cmdRunFinished) apply(c *Conductor, ctx context.Context) {
	r, ok := c.state.running[m.issueID]
	if !ok {
		return
	}
	delete(c.state.running, m.issueID)
	if r.WorkflowState != "" {
		c.state.slotsByState[r.WorkflowState]--
		if c.state.slotsByState[r.WorkflowState] <= 0 {
			delete(c.state.slotsByState, r.WorkflowState)
		}
	}
	// Always release the tracker claim — if the issue is still active
	// on the tracker side, the next tick (or retry due) will re-pick it.
	if relErr := c.tracker.Release(ctx, m.issueID, c.hostMarker); relErr != nil &&
		!errors.Is(relErr, tracker.ErrNotFound) &&
		!errors.Is(relErr, tracker.ErrClaimConflict) {
		c.logger.Warn("conductor: release %s: %v", r.Identifier, relErr)
	}

	if m.err == nil {
		c.logger.Info("conductor: %s finished cleanly (run=%s)", r.Identifier, r.RunID)
		delete(c.state.claimed, m.issueID)
		delete(c.state.retryAttempts, m.issueID)
		c.runAfterRun(ctx, r)
		c.fireSnapshot()
		return
	}

	// Cancellation: leave claim intact only if ctx is shutting down;
	// otherwise release for re-pickup.
	if errors.Is(m.err, context.Canceled) {
		c.logger.Info("conductor: %s cancelled (run=%s)", r.Identifier, r.RunID)
		delete(c.state.claimed, m.issueID)
		c.runAfterRun(ctx, r)
		c.fireSnapshot()
		return
	}

	c.logger.Warn("conductor: %s failed (run=%s): %v", r.Identifier, r.RunID, m.err)
	c.scheduleRetry(m.issueID, r, m.err)
	c.runAfterRun(ctx, r)
	c.fireSnapshot()
}

// cmdRetryDue fires when a retry timer expires.
type cmdRetryDue struct {
	issueID string
}

func (m cmdRetryDue) apply(c *Conductor, ctx context.Context) {
	if t, ok := c.state.retryTimers[m.issueID]; ok {
		t.Stop()
		delete(c.state.retryTimers, m.issueID)
	}
	delete(c.state.claimed, m.issueID)
	// The next tick handles the actual re-dispatch — we just remove
	// the timer and let the polling loop reconsider.
	c.tick(ctx)
}

// cmdCancel cancels an in-flight run from outside (HTTP handler).
type cmdCancel struct {
	issueID string
}

func (m cmdCancel) apply(c *Conductor, _ context.Context) {
	r, ok := c.state.running[m.issueID]
	if !ok {
		return
	}
	if r.Cancel != nil {
		r.Cancel()
	}
	c.logger.Info("conductor: %s cancel requested", r.Identifier)
}
