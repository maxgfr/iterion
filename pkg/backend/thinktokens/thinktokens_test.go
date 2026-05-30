package thinktokens

import "testing"

func TestCount(t *testing.T) {
	if got := Count(""); got != 0 {
		t.Fatalf("empty text: want 0, got %d", got)
	}

	// A non-trivial passage should produce a stable, non-zero count that is
	// far smaller than the raw character length (i.e. real BPE, not 1-per-char).
	text := "Let me reason about this step by step. First, I consider the " +
		"constraints, then I weigh the trade-offs, and finally I commit to a plan."
	got := Count(text)
	if got <= 0 {
		t.Fatalf("expected positive token count, got %d", got)
	}
	if got >= len(text) {
		t.Fatalf("token count %d should be well below char length %d", got, len(text))
	}

	// Determinism: same input → same count.
	if again := Count(text); again != got {
		t.Fatalf("non-deterministic count: %d vs %d", got, again)
	}
}

func TestCountMonotonic(t *testing.T) {
	short := Count("hello world")
	long := Count("hello world hello world hello world hello world")
	if long <= short {
		t.Fatalf("longer text should have more tokens: short=%d long=%d", short, long)
	}
}
