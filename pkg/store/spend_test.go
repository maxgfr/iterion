package store

import (
	"context"
	"testing"
	"time"
)

func TestLoadDailySpendAbsentIsZero(t *testing.T) {
	s := tmpStore(t)
	ds, err := s.LoadDailySpend(context.Background(), "2026-05-30")
	if err != nil {
		t.Fatalf("LoadDailySpend: %v", err)
	}
	if ds.SpentUSD != 0 {
		t.Errorf("absent day SpentUSD = %v, want 0", ds.SpentUSD)
	}
	if ds.Date != "2026-05-30" {
		t.Errorf("Date = %q, want 2026-05-30", ds.Date)
	}
}

func TestAddSpendSumsAcrossRuns(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	const day = "2026-05-30"

	if _, err := s.AddSpend(ctx, day, "run-a", 1.50); err != nil {
		t.Fatalf("AddSpend a: %v", err)
	}
	ds, err := s.AddSpend(ctx, day, "run-b", 2.25)
	if err != nil {
		t.Fatalf("AddSpend b: %v", err)
	}
	if want := 3.75; ds.SpentUSD != want {
		t.Errorf("SpentUSD = %v, want %v", ds.SpentUSD, want)
	}
	// Reload from disk to confirm persistence.
	loaded, err := s.LoadDailySpend(ctx, day)
	if err != nil {
		t.Fatalf("LoadDailySpend: %v", err)
	}
	if loaded.SpentUSD != 3.75 {
		t.Errorf("reloaded SpentUSD = %v, want 3.75", loaded.SpentUSD)
	}
}

func TestAddSpendIdempotentPerRun(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	const day = "2026-05-30"

	// Same run records its growing cumulative cost several times. The
	// day total must reflect the latest cumulative, not the sum of the
	// individual records (which would triple-count).
	for _, c := range []float64{0.10, 0.40, 0.90} {
		if _, err := s.AddSpend(ctx, day, "run-a", c); err != nil {
			t.Fatalf("AddSpend: %v", err)
		}
	}
	ds, _ := s.LoadDailySpend(ctx, day)
	if ds.SpentUSD != 0.90 {
		t.Errorf("SpentUSD = %v, want 0.90 (latest cumulative, not sum)", ds.SpentUSD)
	}

	// A stale lower cumulative (out-of-order write) must not shrink it.
	if _, err := s.AddSpend(ctx, day, "run-a", 0.20); err != nil {
		t.Fatalf("AddSpend stale: %v", err)
	}
	ds, _ = s.LoadDailySpend(ctx, day)
	if ds.SpentUSD != 0.90 {
		t.Errorf("after stale write SpentUSD = %v, want 0.90 (monotonic)", ds.SpentUSD)
	}
}

func TestSetSpendOverrideRoundTrip(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	const day = "2026-05-30"

	ov := &SpendOverride{Active: true, GrantedAt: time.Now().UTC(), GrantedBy: "operator", Note: "ship it"}
	ds, err := s.SetSpendOverride(ctx, day, ov)
	if err != nil {
		t.Fatalf("SetSpendOverride: %v", err)
	}
	if ds.Override == nil || !ds.Override.Active {
		t.Fatalf("Override not active after set: %+v", ds.Override)
	}
	if ds.Override.GrantedBy != "operator" {
		t.Errorf("GrantedBy = %q, want operator", ds.Override.GrantedBy)
	}
	// Override coexists with accumulated spend.
	if _, err := s.AddSpend(ctx, day, "run-a", 5.00); err != nil {
		t.Fatalf("AddSpend: %v", err)
	}
	loaded, _ := s.LoadDailySpend(ctx, day)
	if loaded.Override == nil || !loaded.Override.Active {
		t.Errorf("override lost after AddSpend: %+v", loaded.Override)
	}
	if loaded.SpentUSD != 5.00 {
		t.Errorf("SpentUSD = %v, want 5.00", loaded.SpentUSD)
	}

	// Clearing the override.
	if _, err := s.SetSpendOverride(ctx, day, nil); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	loaded, _ = s.LoadDailySpend(ctx, day)
	if loaded.Override != nil {
		t.Errorf("override not cleared: %+v", loaded.Override)
	}
}

func TestSpendPerDayIsolation(t *testing.T) {
	s := tmpStore(t)
	ctx := context.Background()
	if _, err := s.AddSpend(ctx, "2026-05-30", "run-a", 9.0); err != nil {
		t.Fatalf("AddSpend day1: %v", err)
	}
	ds, err := s.LoadDailySpend(ctx, "2026-05-31")
	if err != nil {
		t.Fatalf("LoadDailySpend day2: %v", err)
	}
	if ds.SpentUSD != 0 {
		t.Errorf("next day SpentUSD = %v, want 0 (per-day isolation)", ds.SpentUSD)
	}
}

// Compile-time assertion that FilesystemRunStore satisfies SpendStore.
var _ SpendStore = (*FilesystemRunStore)(nil)
