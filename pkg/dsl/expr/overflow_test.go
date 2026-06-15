package expr

import (
	"math"
	"testing"
)

// TestArithIntegerDivisionOverflow guards the MinInt64 / -1 case: it overflows
// int64, and Go's spec defines MinInt64 / -1 == MinInt64 (no panic), so without
// an explicit guard arith would silently return the wrong value.
func TestArithIntegerDivisionOverflow(t *testing.T) {
	if _, err := arith("/", int64(math.MinInt64), int64(-1)); err == nil {
		t.Fatalf("arith(/, MinInt64, -1): want overflow error, got nil")
	}
	// A normal division still works.
	got, err := arith("/", int64(6), int64(-2))
	if err != nil {
		t.Fatalf("arith(/, 6, -2): unexpected error: %v", err)
	}
	if got != int64(-3) {
		t.Fatalf("arith(/, 6, -2) = %v, want -3", got)
	}
}

// TestMulCheckedInt64Overflow guards the MinInt64 * -1 case, which the
// r/b != a heuristic alone misses (MinInt64 / -1 == MinInt64 fools it).
func TestMulCheckedInt64Overflow(t *testing.T) {
	cases := []struct {
		a, b int64
		ok   bool
	}{
		{math.MinInt64, -1, false},
		{-1, math.MinInt64, false},
		{math.MaxInt64, 2, false},
		{1 << 40, 1 << 40, false},
		{6, 7, true},
		{0, math.MinInt64, true},
		{math.MinInt64, 1, true},
	}
	for _, c := range cases {
		got, ok := mulCheckedInt64(c.a, c.b)
		if ok != c.ok {
			t.Errorf("mulCheckedInt64(%d, %d) ok = %v, want %v (got %d)", c.a, c.b, ok, c.ok, got)
		}
		if c.ok && got != c.a*c.b {
			t.Errorf("mulCheckedInt64(%d, %d) = %d, want %d", c.a, c.b, got, c.a*c.b)
		}
	}
}
