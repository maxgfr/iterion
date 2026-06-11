package orgusage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// runCounterSuite exercises Counter semantics shared by both
// implementations so the memory variant stays in lock-step with Mongo.
func runCounterSuite(t *testing.T, c Counter) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	t.Run("unlimited still meters", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			ok, err := c.AllowRun(ctx, "t-unlimited", now, 0)
			if err != nil || !ok {
				t.Fatalf("AllowRun #%d: ok=%v err=%v", i, ok, err)
			}
		}
		u, err := c.Usage(ctx, "t-unlimited", now)
		if err != nil {
			t.Fatalf("Usage: %v", err)
		}
		if u.Runs != 3 {
			t.Fatalf("Runs = %d, want 3", u.Runs)
		}
	})

	t.Run("cap denies without consuming", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if ok, err := c.AllowRun(ctx, "t-capped", now, 2); err != nil || !ok {
				t.Fatalf("AllowRun #%d: ok=%v err=%v", i, ok, err)
			}
		}
		ok, err := c.AllowRun(ctx, "t-capped", now, 2)
		if err != nil {
			t.Fatalf("AllowRun denied: %v", err)
		}
		if ok {
			t.Fatal("AllowRun = true past the cap")
		}
		u, _ := c.Usage(ctx, "t-capped", now)
		if u.Runs != 2 {
			t.Fatalf("denied call consumed quota: Runs = %d, want 2", u.Runs)
		}
	})

	t.Run("months are disjoint buckets", func(t *testing.T) {
		if ok, _ := c.AllowRun(ctx, "t-months", now, 1); !ok {
			t.Fatal("first month launch denied")
		}
		nextMonth := now.AddDate(0, 1, 0)
		ok, err := c.AllowRun(ctx, "t-months", nextMonth, 1)
		if err != nil || !ok {
			t.Fatalf("next month launch: ok=%v err=%v", ok, err)
		}
		u, _ := c.Usage(ctx, "t-months", now)
		if u.Runs != 1 {
			t.Fatalf("first month Runs = %d, want 1", u.Runs)
		}
	})

	t.Run("tenants are isolated", func(t *testing.T) {
		if ok, _ := c.AllowRun(ctx, "t-a", now, 1); !ok {
			t.Fatal("t-a launch denied")
		}
		if ok, _ := c.AllowRun(ctx, "t-b", now, 1); !ok {
			t.Fatal("t-b denied by t-a's consumption")
		}
	})

	t.Run("spend accumulates", func(t *testing.T) {
		if err := c.AddSpend(ctx, "t-spend", now, 1.234, 1000, 200); err != nil {
			t.Fatalf("AddSpend: %v", err)
		}
		if err := c.AddSpend(ctx, "t-spend", now, 0.766, 500, 100); err != nil {
			t.Fatalf("AddSpend: %v", err)
		}
		// Zero-valued spend must be a no-op, not an error.
		if err := c.AddSpend(ctx, "t-spend", now, 0, 0, 0); err != nil {
			t.Fatalf("AddSpend zero: %v", err)
		}
		u, err := c.Usage(ctx, "t-spend", now)
		if err != nil {
			t.Fatalf("Usage: %v", err)
		}
		if u.CostUSD != 2.0 {
			t.Fatalf("CostUSD = %v, want 2.0", u.CostUSD)
		}
		if u.InputTokens != 1500 || u.OutputTokens != 300 {
			t.Fatalf("tokens = %d/%d, want 1500/300", u.InputTokens, u.OutputTokens)
		}
	})

	t.Run("empty month reads zero", func(t *testing.T) {
		u, err := c.Usage(ctx, "t-never-seen", now)
		if err != nil {
			t.Fatalf("Usage: %v", err)
		}
		if u.Runs != 0 || u.CostUSD != 0 {
			t.Fatalf("expected zero usage, got %+v", u)
		}
		if u.Month != "2026-06" {
			t.Fatalf("Month = %q, want 2026-06", u.Month)
		}
	})

	t.Run("concurrent launches never overshoot the cap", func(t *testing.T) {
		const cap = 10
		const callers = 40
		var wg sync.WaitGroup
		var mu sync.Mutex
		allowed := 0
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := c.AllowRun(ctx, "t-race", now, cap)
				if err != nil {
					t.Errorf("AllowRun: %v", err)
					return
				}
				if ok {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if allowed > cap {
			t.Fatalf("allowed %d launches past cap %d", allowed, cap)
		}
		u, _ := c.Usage(ctx, "t-race", now)
		if u.Runs > cap {
			t.Fatalf("metered Runs = %d past cap %d", u.Runs, cap)
		}
	})
}

func TestMemoryCounter(t *testing.T) {
	runCounterSuite(t, NewMemoryCounter())
}

func TestCostMillisRoundTrip(t *testing.T) {
	cases := []struct {
		usd  float64
		want int64
	}{
		{0, 0}, {-1, 0}, {0.0004, 0}, {0.0005, 1}, {1.234, 1234}, {2.7185, 2719},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c.usd), func(t *testing.T) {
			if got := costToMillis(c.usd); got != c.want {
				t.Fatalf("costToMillis(%v) = %d, want %d", c.usd, got, c.want)
			}
		})
	}
}
