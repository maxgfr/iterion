package clock

import (
	"testing"
	"time"
)

func TestRealClockUTC(t *testing.T) {
	got := RealClock{}.Now()
	if got.Location() != time.UTC {
		t.Errorf("RealClock.Now location = %v, want UTC", got.Location())
	}
}

func TestFakeClockSetAndAdvance(t *testing.T) {
	base := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	c := NewFakeClock(base)
	if !c.Now().Equal(base) {
		t.Fatalf("Now = %v, want %v", c.Now(), base)
	}
	c.Advance(2 * time.Hour)
	if want := base.Add(2 * time.Hour); !c.Now().Equal(want) {
		t.Errorf("after Advance: Now = %v, want %v", c.Now(), want)
	}
	next := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c.SetTime(next)
	if !c.Now().Equal(next) {
		t.Errorf("after SetTime: Now = %v, want %v", c.Now(), next)
	}
}

func TestFakeClockNormalisesToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+5", 5*3600)
	c := NewFakeClock(time.Date(2026, 5, 30, 10, 0, 0, 0, loc))
	if c.Now().Location() != time.UTC {
		t.Errorf("FakeClock.Now location = %v, want UTC", c.Now().Location())
	}
}

func TestDayKey(t *testing.T) {
	// A timestamp late in the UTC day stays on that day; the same instant
	// in a +14 zone would be the next calendar day locally, but DayKey is
	// UTC-anchored so the key is stable across hosts.
	tm := time.Date(2026, 5, 30, 23, 59, 0, 0, time.UTC)
	if got := DayKey(tm); got != "2026-05-30" {
		t.Errorf("DayKey = %q, want 2026-05-30", got)
	}
	// One minute later crosses into the next UTC day.
	if got := DayKey(tm.Add(2 * time.Minute)); got != "2026-05-31" {
		t.Errorf("DayKey after rollover = %q, want 2026-05-31", got)
	}
}
