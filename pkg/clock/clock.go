// Package clock provides a small Clock abstraction so time-dependent
// logic (notably the per-day spend-cap reset) can be driven by a fake
// clock in tests. Production code uses RealClock (UTC); tests inject
// FakeClock and advance it across day boundaries deterministically.
//
// The rest of the codebase calls time.Now() directly; this package is
// intentionally narrow and only adopted where day-boundary behaviour
// must be tested without sleeping or racing the wall clock.
package clock

import (
	"sync"
	"time"
)

// Clock is the minimal time source consumed by the spend-cap engine.
type Clock interface {
	// Now returns the current time. Implementations SHOULD return UTC
	// so callers can derive a stable YYYY-MM-DD day key.
	Now() time.Time
}

// RealClock is the production Clock; it returns the wall-clock time in
// UTC so day keys are stable regardless of host timezone.
type RealClock struct{}

// Now returns time.Now().UTC().
func (RealClock) Now() time.Time { return time.Now().UTC() }

// Default is the package-level Clock used when no clock is injected.
var Default Clock = RealClock{}

// FakeClock is a deterministic Clock for tests. The zero value is not
// usable — construct with NewFakeClock.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock pinned to t (normalised to UTC).
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t.UTC()}
}

// Now returns the fake clock's current time.
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// SetTime pins the fake clock to t (normalised to UTC).
func (f *FakeClock) SetTime(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}

// Advance moves the fake clock forward by d.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// DayKey returns the UTC calendar-day key (YYYY-MM-DD) for t. This is
// the canonical partition key for the daily spend ledger.
func DayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}
