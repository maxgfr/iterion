package dispatcher

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudPublishingRunner_BlocksUntilTerminal(t *testing.T) {
	var polls int32
	r := &CloudPublishingRunner{
		Interval: time.Millisecond,
		LaunchFn: func(context.Context, DispatchSpec) (string, error) { return "run-1", nil },
		PollFn: func(context.Context, string) (bool, error) {
			// Not done for the first two polls, then done+success.
			if atomic.AddInt32(&polls, 1) < 3 {
				return false, nil
			}
			return true, nil
		},
	}
	if err := r.Dispatch(context.Background(), DispatchSpec{}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if atomic.LoadInt32(&polls) < 3 {
		t.Errorf("expected to poll until terminal, polls=%d", polls)
	}
}

func TestCloudPublishingRunner_ReturnsTerminalError(t *testing.T) {
	want := errors.New("run failed")
	r := &CloudPublishingRunner{
		Interval: time.Millisecond,
		LaunchFn: func(context.Context, DispatchSpec) (string, error) { return "run-1", nil },
		PollFn:   func(context.Context, string) (bool, error) { return true, want },
	}
	if err := r.Dispatch(context.Background(), DispatchSpec{}); !errors.Is(err, want) {
		t.Fatalf("want terminal error, got %v", err)
	}
}

func TestCloudPublishingRunner_LaunchErrorShortCircuits(t *testing.T) {
	want := errors.New("enqueue failed")
	var polled bool
	r := &CloudPublishingRunner{
		LaunchFn: func(context.Context, DispatchSpec) (string, error) { return "", want },
		PollFn:   func(context.Context, string) (bool, error) { polled = true; return true, nil },
	}
	if err := r.Dispatch(context.Background(), DispatchSpec{}); !errors.Is(err, want) {
		t.Fatalf("want launch error, got %v", err)
	}
	if polled {
		t.Error("must not poll when launch fails")
	}
}

func TestCloudPublishingRunner_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &CloudPublishingRunner{
		Interval: 10 * time.Millisecond,
		LaunchFn: func(context.Context, DispatchSpec) (string, error) { return "run-1", nil },
		PollFn: func(context.Context, string) (bool, error) {
			cancel() // cancel after the first poll; run never terminates
			return false, nil
		},
	}
	if err := r.Dispatch(ctx, DispatchSpec{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestCloudPublishingRunner_RequiresFuncs(t *testing.T) {
	if err := (&CloudPublishingRunner{}).Dispatch(context.Background(), DispatchSpec{}); err == nil {
		t.Error("expected error when LaunchFn/PollFn unset")
	}
}
