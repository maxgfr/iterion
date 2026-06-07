package model

import (
	"errors"
	"testing"
)

func TestEffectiveMaxAttempts(t *testing.T) {
	rp := RetryPolicy{} // defaults: 3 standard, 6 transient
	if got := rp.effectiveMaxAttempts(errors.New("fetch failed")); got != DefaultMaxAttemptsTransient {
		t.Fatalf("network err: effectiveMaxAttempts = %d, want %d", got, DefaultMaxAttemptsTransient)
	}
	if got := rp.effectiveMaxAttempts(errors.New("exit status 137")); got != DefaultMaxAttempts {
		t.Fatalf("non-network (signal) err: effectiveMaxAttempts = %d, want %d", got, DefaultMaxAttempts)
	}
	if got := rp.effectiveMaxAttempts(nil); got != DefaultMaxAttempts {
		t.Fatalf("nil err: effectiveMaxAttempts = %d, want %d", got, DefaultMaxAttempts)
	}
}

func TestMaxAttemptsTransientNeverSmallerThanStandard(t *testing.T) {
	rp := RetryPolicy{MaxAttempts: 10, MaxAttemptsTransient: 2}
	if got := rp.maxAttemptsTransient(); got != 10 {
		t.Fatalf("transient budget must clamp up to standard: got %d, want 10", got)
	}
}

func TestIsDelegateRetryableNetwork(t *testing.T) {
	// A connectivity failure stringified through the claude_code wrapper.
	netErr := errors.New("delegate: claude-code failed: claude session ended without result message; fetch failed")
	if !isDelegateRetryable(netErr) {
		t.Fatal("expected network-tagged delegate error to be retryable")
	}
	// A genuine permanent failure (no network marker, non-signal exit) stays
	// non-retryable — we must not loop forever on a real error.
	permanent := errors.New("delegate: claude-code failed: claude session ended without result message (cli_exit_code=2)")
	if isDelegateRetryable(permanent) {
		t.Fatal("expected bare exit-2 to be non-retryable")
	}
}

func TestIsRetryableRawNetError(t *testing.T) {
	if !isRetryable(errors.New("dial tcp: lookup api.anthropic.com: no such host")) {
		t.Fatal("expected raw net error to be retryable for the claw backend")
	}
	if isRetryable(errors.New("missing required field stats")) {
		t.Fatal("schema validation error must not be retryable")
	}
}
