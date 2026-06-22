package cloudsched

import (
	"context"
	"time"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// LaunchFunc fires one scheduled bot run. The cloud bootstrap wires it to the
// run publisher (resolve the bot, build a LaunchSpec, SubmitLaunch).
type LaunchFunc func(ctx context.Context, sb ScheduledBot) error

// Ticker fires due schedules. It is multi-replica-safe WITHOUT leader
// election: every replica may run a Ticker; the CAS in ClaimTick guarantees
// each slot fires exactly once (the first replica to advance next_fire_at
// wins; the rest see the moved value and skip).
type Ticker struct {
	Store    Store
	Launch   LaunchFunc
	Interval time.Duration // default 1 minute
	Logger   *iterlog.Logger
	// Now is injectable for tests; defaults to time.Now().UTC().
	Now func() time.Time
}

func (t *Ticker) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now().UTC()
}

// Tick fires every due schedule this caller wins the CAS for, returning the
// count it fired. Exposed for tests + a manual kick.
func (t *Ticker) Tick(ctx context.Context) (int, error) {
	now := t.now()
	due, err := t.Store.ListDue(ctx, now, 200)
	if err != nil {
		return 0, err
	}
	fired := 0
	for _, sb := range due {
		next, nerr := NextFire(sb.Cron, now)
		if nerr != nil {
			t.warn("bad cron on %s: %v", sb.ID, nerr)
			continue
		}
		won, cerr := t.Store.ClaimTick(ctx, sb.ID, sb.NextFireAt, next, now)
		if cerr != nil {
			t.warn("claim %s: %v", sb.ID, cerr)
			continue
		}
		if !won {
			continue // another replica claimed this slot
		}
		// The slot is already advanced; a failed launch is logged, not
		// retried within the slot (the next slot fires normally). This is
		// at-most-once-per-slot, matching the host-crontab scheduler.
		if lerr := t.Launch(ctx, sb); lerr != nil {
			t.warn("launch %s (%s): %v", sb.ID, sb.BotID, lerr)
		}
		fired++
	}
	return fired, nil
}

// Run loops Tick every Interval until ctx is cancelled. Start one per replica.
func (t *Ticker) Run(ctx context.Context) {
	interval := t.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	tk := time.NewTicker(interval)
	defer tk.Stop()
	for {
		if _, err := t.Tick(ctx); err != nil {
			t.warn("tick: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
	}
}

func (t *Ticker) warn(format string, args ...any) {
	if t.Logger != nil {
		t.Logger.Warn("cloudsched: "+format, args...)
	}
}
