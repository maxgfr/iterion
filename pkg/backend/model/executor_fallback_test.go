package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// providerScriptedBackend is a delegate.Backend whose behaviour is keyed
// on task.ProviderHint, so a test can make one provider fail and another
// succeed. It records the provider hint of every Execute call in order,
// which lets tests assert how far the executor walked the chain (and how
// many retry attempts each provider got). Calls are synchronous from the
// executor's goroutine, so no locking is needed.
type providerScriptedBackend struct {
	calls []string         // ProviderHint of each Execute call, in order
	fail  map[string]error // provider hint -> error to return (absent = success)
}

func (b *providerScriptedBackend) Execute(_ context.Context, task delegate.Task) (delegate.Result, error) {
	b.calls = append(b.calls, task.ProviderHint)
	if err, ok := b.fail[task.ProviderHint]; ok {
		return delegate.Result{}, err
	}
	return delegate.Result{
		Output:      map[string]interface{}{"ok": true, "served_by": task.ProviderHint},
		BackendName: delegate.BackendClaudeCode,
	}, nil
}

// fallbackRecorder captures OnProviderFallback invocations so tests can
// assert exactly one log-note-equivalent fired per fall-through.
type fallbackRecorder struct {
	events []ProviderFallbackInfo
}

func (r *fallbackRecorder) hook() EventHooks {
	return EventHooks{
		OnProviderFallback: func(_ string, info ProviderFallbackInfo) {
			r.events = append(r.events, info)
		},
	}
}

func newFallbackExecutor(reg *delegate.Registry, hooks EventHooks) *ClawExecutor {
	return &ClawExecutor{
		retry:           RetryPolicy{MaxAttempts: 2, BackoffBase: time.Millisecond},
		logger:          iterlog.Nop(),
		backendRegistry: reg,
		hooks:           hooks,
	}
}

func fallbackAgentNode(id, backend, provider string) *ir.AgentNode {
	n := &ir.AgentNode{}
	n.ID = id
	n.LLMFields.Backend = backend
	n.LLMFields.Provider = provider
	return n
}

// TestProviderFallback_FallsThroughThenSucceeds is the headline case:
// a claude_code node declares `provider: "zai,anthropic"`, z.ai fails
// beyond its retry budget, and the run transparently completes against
// anthropic with a single fall-through note.
func TestProviderFallback_FallsThroughThenSucceeds(t *testing.T) {
	rec := &fallbackRecorder{}
	reg := delegate.NewRegistry()
	fake := &providerScriptedBackend{fail: map[string]error{
		"zai": &delegate.ErrTransient{Reason: "z.ai overloaded"},
	}}
	reg.Register(delegate.BackendClaudeCode, fake)

	e := newFallbackExecutor(reg, rec.hook())
	node := fallbackAgentNode("review", delegate.BackendClaudeCode, "zai,anthropic")

	out, err := e.Execute(context.Background(), node, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected success after fall-through, got error: %v", err)
	}
	if out["served_by"] != "anthropic" {
		t.Errorf("expected output served by anthropic, got served_by=%v", out["served_by"])
	}
	// zai retried up to the budget (2 attempts), then anthropic once.
	wantCalls := []string{"zai", "zai", "anthropic"}
	if !equalStrings(fake.calls, wantCalls) {
		t.Errorf("provider call order = %v, want %v", fake.calls, wantCalls)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly one OnProviderFallback, got %d: %+v", len(rec.events), rec.events)
	}
	if rec.events[0].From != "zai" || rec.events[0].To != "anthropic" {
		t.Errorf("fallback event = %+v, want from=zai to=anthropic", rec.events[0])
	}
}

// TestProviderFallback_WholeChainFails verifies that when every provider
// in the chain fails, the run fails (it is not silently swallowed) and the
// surfaced error names the chain that was attempted.
func TestProviderFallback_WholeChainFails(t *testing.T) {
	rec := &fallbackRecorder{}
	reg := delegate.NewRegistry()
	fake := &providerScriptedBackend{fail: map[string]error{
		"zai":       &delegate.ErrTransient{Reason: "z.ai overloaded"},
		"anthropic": &delegate.ErrTransient{Reason: "anthropic overloaded"},
	}}
	reg.Register(delegate.BackendClaudeCode, fake)

	e := newFallbackExecutor(reg, rec.hook())
	node := fallbackAgentNode("review", delegate.BackendClaudeCode, "zai,anthropic")

	_, err := e.Execute(context.Background(), node, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !strings.Contains(err.Error(), "chain") || !strings.Contains(err.Error(), "zai") {
		t.Errorf("error should name the exhausted chain, got: %v", err)
	}
	// One fall-through (zai -> anthropic); the final provider's failure is
	// the terminal error, not another fall-through.
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly one OnProviderFallback, got %d", len(rec.events))
	}
	wantCalls := []string{"zai", "zai", "anthropic", "anthropic"}
	if !equalStrings(fake.calls, wantCalls) {
		t.Errorf("provider call order = %v, want %v", fake.calls, wantCalls)
	}
}

// TestProviderFallback_SingleValueNeverFallsThrough guards back-compat: a
// single-value provider (the historical form) runs exactly once and never
// fires the fall-through machinery.
func TestProviderFallback_SingleValueNeverFallsThrough(t *testing.T) {
	rec := &fallbackRecorder{}
	reg := delegate.NewRegistry()
	fake := &providerScriptedBackend{}
	reg.Register(delegate.BackendClaudeCode, fake)

	e := newFallbackExecutor(reg, rec.hook())
	node := fallbackAgentNode("review", delegate.BackendClaudeCode, "anthropic")

	out, err := e.Execute(context.Background(), node, map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["served_by"] != "anthropic" {
		t.Errorf("served_by=%v, want anthropic", out["served_by"])
	}
	if len(fake.calls) != 1 {
		t.Errorf("expected 1 call, got %d (%v)", len(fake.calls), fake.calls)
	}
	if len(rec.events) != 0 {
		t.Errorf("single-value provider must not fall through, got %d events", len(rec.events))
	}
}

// TestProviderFallback_NonRetryableStillFallsThrough verifies that a hard
// (non-retryable) error — e.g. an auth failure on the first provider —
// also triggers a fall-through, without burning the retry budget first.
func TestProviderFallback_NonRetryableStillFallsThrough(t *testing.T) {
	rec := &fallbackRecorder{}
	reg := delegate.NewRegistry()
	fake := &providerScriptedBackend{fail: map[string]error{
		"zai": errors.New("exit status 1"), // application error, not retryable
	}}
	reg.Register(delegate.BackendClaudeCode, fake)

	e := newFallbackExecutor(reg, rec.hook())
	node := fallbackAgentNode("review", delegate.BackendClaudeCode, "zai,anthropic")

	out, err := e.Execute(context.Background(), node, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected success after fall-through, got: %v", err)
	}
	if out["served_by"] != "anthropic" {
		t.Errorf("served_by=%v, want anthropic", out["served_by"])
	}
	// zai called once (no retry on a non-retryable error), then anthropic.
	wantCalls := []string{"zai", "anthropic"}
	if !equalStrings(fake.calls, wantCalls) {
		t.Errorf("provider call order = %v, want %v", fake.calls, wantCalls)
	}
	if len(rec.events) != 1 {
		t.Errorf("expected exactly one OnProviderFallback, got %d", len(rec.events))
	}
}

// TestDispatchProviderFallback_ContextCancelledNoFallthrough verifies that
// a cancelled/timed-out context aborts the chain immediately rather than
// thrashing through every remaining provider.
func TestDispatchProviderFallback_ContextCancelledNoFallthrough(t *testing.T) {
	rec := &fallbackRecorder{}
	e := newFallbackExecutor(nil, rec.hook())
	fake := &providerScriptedBackend{fail: map[string]error{
		"zai": &delegate.ErrTransient{Reason: "z.ai overloaded"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	task := delegate.Task{NodeID: "n"}
	_, err := e.dispatchWithProviderFallback(ctx, "n", delegate.BackendClaudeCode, []string{"zai", "anthropic"}, fake, &task)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	// zai was attempted (and its retry backoff observed the cancellation);
	// anthropic must never be reached.
	for _, p := range fake.calls {
		if p == "anthropic" {
			t.Fatalf("anthropic should not be tried after cancellation; calls=%v", fake.calls)
		}
	}
	if len(rec.events) != 0 {
		t.Errorf("cancellation must not emit a fall-through, got %d", len(rec.events))
	}
}

// TestDispatchProviderFallback_CollapsesChainForHintIgnoringBackend
// verifies the eligibility guard: claw ignores the provider hint, so a
// multi-element chain must collapse to the head provider (a single
// attempt) rather than wastefully re-running the identical call.
func TestDispatchProviderFallback_CollapsesChainForHintIgnoringBackend(t *testing.T) {
	rec := &fallbackRecorder{}
	e := newFallbackExecutor(nil, rec.hook())
	fake := &providerScriptedBackend{} // no failures

	task := delegate.Task{NodeID: "n"}
	out, err := e.dispatchWithProviderFallback(context.Background(), "n", delegate.BackendClaw, []string{"zai", "anthropic"}, fake, &task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "zai" {
		t.Errorf("claw chain should collapse to one attempt of the head provider; calls=%v", fake.calls)
	}
	if out.Output["served_by"] != "zai" {
		t.Errorf("served_by=%v, want zai", out.Output["served_by"])
	}
	if len(rec.events) != 0 {
		t.Errorf("collapsed chain must not fall through, got %d events", len(rec.events))
	}
}

func TestChainHooks_ComposesProviderFallback(t *testing.T) {
	var calls []string
	chained := ChainHooks(
		EventHooks{OnProviderFallback: func(nodeID string, info ProviderFallbackInfo) {
			calls = append(calls, "a:"+nodeID+":"+info.From+":"+info.To)
		}},
		EventHooks{OnProviderFallback: func(nodeID string, info ProviderFallbackInfo) {
			calls = append(calls, "b:"+nodeID+":"+info.From+":"+info.To)
		}},
	)

	if chained.OnProviderFallback == nil {
		t.Fatal("expected chained OnProviderFallback to be installed")
	}
	chained.OnProviderFallback("review", ProviderFallbackInfo{From: "zai", To: "anthropic"})

	want := []string{"a:review:zai:anthropic", "b:review:zai:anthropic"}
	if !equalStrings(calls, want) {
		t.Fatalf("chained OnProviderFallback calls = %v, want %v", calls, want)
	}
}

func TestProviderFallback_NonRetryableWithNilLoggerStillFallsThrough(t *testing.T) {
	rec := &fallbackRecorder{}
	reg := delegate.NewRegistry()
	fake := &providerScriptedBackend{fail: map[string]error{
		"zai": errors.New("exit status 1"), // application error, not retryable
	}}
	reg.Register(delegate.BackendClaudeCode, fake)

	// Deliberately omit logger, matching a ClawExecutor constructed
	// without WithLogger. A non-retryable first-provider failure should
	// still fall through and fire hooks instead of panicking while logging.
	e := &ClawExecutor{
		retry:           RetryPolicy{MaxAttempts: 2, BackoffBase: time.Millisecond},
		backendRegistry: reg,
		hooks:           rec.hook(),
	}
	node := fallbackAgentNode("review", delegate.BackendClaudeCode, "zai,anthropic")

	out, err := e.Execute(context.Background(), node, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected success after fall-through with nil logger, got: %v", err)
	}
	if out["served_by"] != "anthropic" {
		t.Errorf("served_by=%v, want anthropic", out["served_by"])
	}
	wantCalls := []string{"zai", "anthropic"}
	if !equalStrings(fake.calls, wantCalls) {
		t.Errorf("provider call order = %v, want %v", fake.calls, wantCalls)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly one OnProviderFallback, got %d", len(rec.events))
	}
	if rec.events[0].From != "zai" || rec.events[0].To != "anthropic" {
		t.Errorf("fallback event = %+v, want from=zai to=anthropic", rec.events[0])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
