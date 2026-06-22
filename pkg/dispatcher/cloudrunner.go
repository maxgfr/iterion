package dispatcher

import (
	"context"
	"errors"
	"time"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// CloudPublishingRunner is the cloud-mode Runner: instead of executing the
// engine in-process (EngineRunner), it ENQUEUES the run through the cloud
// publisher and then BLOCKS — polling the run store — until the run reaches a
// terminal or paused status, so the dispatcher actor's claim/retry/state
// machinery works unchanged against runs that actually execute in runner pods.
//
// The two halves that touch the cloud stack (the launch + the status read) are
// injected as funcs so the runner stays decoupled from runview/store and is
// unit-testable with fakes. The cloud bootstrap (pkg/cloud) wires:
//   - LaunchFn: resolve the bot for spec.Assignee, build a runview.LaunchSpec,
//     call the publisher (SubmitLaunch), return the run id.
//   - PollFn:   read the run record; report (done, runErr) — done=true once the
//     status is terminal or paused, runErr nil on success, the run's terminal
//     error (or a paused sentinel) otherwise. The dispatcher reads the
//     PERSISTED status for resume decisions, so any non-nil error drives retry.
type CloudPublishingRunner struct {
	// LaunchFn enqueues the run and returns its id.
	LaunchFn func(ctx context.Context, spec DispatchSpec) (runID string, err error)
	// PollFn reports whether the run is done (terminal/paused) and, if so, the
	// error to surface (nil = finished cleanly).
	PollFn func(ctx context.Context, runID string) (done bool, runErr error)
	// Interval is the poll cadence (defaults to 3s when unset).
	Interval time.Duration
	Logger   *iterlog.Logger
}

// Close satisfies ManagedRunner (no resources to release).
func (r *CloudPublishingRunner) Close() error { return nil }

// Dispatch implements Runner: enqueue, then block until the run terminates.
func (r *CloudPublishingRunner) Dispatch(ctx context.Context, spec DispatchSpec) error {
	if r.LaunchFn == nil || r.PollFn == nil {
		return errors.New("cloud runner: LaunchFn and PollFn are required")
	}
	runID, err := r.LaunchFn(ctx, spec)
	if err != nil {
		return err
	}
	if r.Logger != nil {
		r.Logger.Debug("cloud runner: enqueued %s for issue %s, awaiting completion", runID, issueIDOf(spec))
	}
	interval := r.Interval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// Poll once immediately on entry too, so a fast run isn't held for a
		// full interval.
		done, runErr := r.PollFn(ctx, runID)
		if done {
			return runErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func issueIDOf(spec DispatchSpec) string {
	if spec.Issue != nil {
		return spec.Issue.ID
	}
	return ""
}
