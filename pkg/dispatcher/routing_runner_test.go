package dispatcher

import (
	"context"
	"errors"
	"testing"
)

// recordingRunner records every spec it sees so tests can assert on
// which Runner received which dispatch.
type recordingRunner struct {
	name  string
	seen  []DispatchSpec
	err   error
	close bool
}

func (r *recordingRunner) Dispatch(_ context.Context, spec DispatchSpec) error {
	r.seen = append(r.seen, spec)
	return r.err
}

func (r *recordingRunner) Close() error {
	r.close = true
	return nil
}

func TestRoutingRunnerPicksByAssignee(t *testing.T) {
	def := &recordingRunner{name: "default"}
	vfd := &recordingRunner{name: "feature_dev"}
	rev := &recordingRunner{name: "whole_improve_loop"}

	rr := &RoutingRunner{
		Default: def,
		ByAssignee: map[string]Runner{
			"feature_dev":        vfd,
			"whole_improve_loop": rev,
		},
	}

	cases := []struct {
		assignee string
		want     *recordingRunner
	}{
		{"feature_dev", vfd},
		{"whole_improve_loop", rev},
		{"", def},            // empty → default
		{"unknown-bot", def}, // genuine miss → default
		{"FEATURE_DEV", vfd}, // case-insensitive → feature_dev (NormalizeName)
		{"feature-dev", vfd}, // kebab tolerated against snake key → feature_dev
	}
	for _, tc := range cases {
		t.Run(tc.assignee, func(t *testing.T) {
			err := rr.Dispatch(context.Background(), DispatchSpec{
				RunID:    "r-" + tc.assignee,
				Assignee: tc.assignee,
			})
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if n := len(tc.want.seen); n == 0 {
				t.Fatalf("expected runner %q to receive dispatch, got 0", tc.want.name)
			}
			last := tc.want.seen[len(tc.want.seen)-1]
			if last.Assignee != tc.assignee {
				t.Fatalf("runner %q saw assignee %q, want %q", tc.want.name, last.Assignee, tc.assignee)
			}
		})
	}

	// default got only the genuine fallbacks (empty + unknown-bot); the
	// case/kebab variants now resolve to feature_dev via NormalizeName.
	if got := len(def.seen); got != 2 {
		t.Errorf("default runner saw %d dispatches, want 2", got)
	}
}

// TestRoutingRunnerHasRoute pins the route-existence check the
// dispatch() guard uses to refuse an explicit bot that would otherwise
// silently fall through to the default workflow.
func TestRoutingRunnerHasRoute(t *testing.T) {
	rr := &RoutingRunner{
		Default:    &recordingRunner{name: "default"},
		ByAssignee: map[string]Runner{"feature_dev": &recordingRunner{name: "feature_dev"}},
	}
	cases := []struct {
		key  string
		want bool
	}{
		{"feature_dev", true}, // exact
		{"feature-dev", true}, // kebab → snake key
		{"FEATURE_DEV", true}, // case-insensitive
		{"ghost", false},      // no route
		{"", false},           // empty
	}
	for _, tc := range cases {
		if got := rr.HasRoute(tc.key); got != tc.want {
			t.Errorf("HasRoute(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestRoutingRunnerEmptyMapDelegatesToDefault(t *testing.T) {
	def := &recordingRunner{name: "default"}
	rr := &RoutingRunner{Default: def}

	if err := rr.Dispatch(context.Background(), DispatchSpec{RunID: "r1", Assignee: "anyone"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(def.seen) != 1 {
		t.Fatalf("default should have received exactly 1 dispatch, got %d", len(def.seen))
	}
}

func TestRoutingRunnerNilDefaultIsError(t *testing.T) {
	rr := &RoutingRunner{}
	err := rr.Dispatch(context.Background(), DispatchSpec{RunID: "r1"})
	if err == nil {
		t.Fatal("expected error for nil default runner, got nil")
	}
}

func TestRoutingRunnerDispatchPropagatesRunnerError(t *testing.T) {
	wantErr := errors.New("boom")
	def := &recordingRunner{name: "default", err: wantErr}
	rr := &RoutingRunner{Default: def}
	err := rr.Dispatch(context.Background(), DispatchSpec{RunID: "r1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped or returned %v, got %v", wantErr, err)
	}
}

// varReportingRunner is a recordingRunner that also reports a fixed
// declared-var set, so RoutingRunner.DeclaredVars routing can be tested
// without compiling a real workflow.
type varReportingRunner struct {
	recordingRunner
	vars map[string]struct{}
}

func (r *varReportingRunner) DeclaredVars(string) map[string]struct{} { return r.vars }

// TestRoutingRunnerDeclaredVars pins that DeclaredVars resolves the same
// route as Dispatch and forwards the routed runner's declared vars — the
// dispatcher uses it to warn when a per-ticket bot_arg names a var the
// routed workflow doesn't declare (such args are silently dropped at
// runtime). A route whose runner can't report vars yields nil so buildSpec
// fails open (skips validation) rather than warning on everything.
func TestRoutingRunnerDeclaredVars(t *testing.T) {
	fd := &varReportingRunner{
		recordingRunner: recordingRunner{name: "feature_dev"},
		vars:            map[string]struct{}{"loop_cap": {}, "workspace_dir": {}},
	}
	def := &recordingRunner{name: "default"} // no DeclaredVars method
	rr := &RoutingRunner{
		Default:    def,
		ByAssignee: map[string]Runner{"feature_dev": fd},
	}

	v := rr.DeclaredVars("feature_dev")
	if _, ok := v["loop_cap"]; !ok {
		t.Fatalf("DeclaredVars(feature_dev) = %v, want loop_cap present", v)
	}
	if _, ok := v["not_declared"]; ok {
		t.Errorf("DeclaredVars must not invent undeclared keys: %v", v)
	}
	// kebab/case normalisation routes the same as Dispatch/HasRoute.
	if v := rr.DeclaredVars("feature-dev"); v == nil {
		t.Errorf("DeclaredVars(feature-dev) should route to feature_dev via NormalizeName")
	}
	// Unknown route → falls back to Default, which can't report vars → nil.
	if v := rr.DeclaredVars("ghost"); v != nil {
		t.Fatalf("DeclaredVars(ghost) = %v, want nil (default runner reports nothing)", v)
	}
}

func TestRoutingRunnerCloseClosesAllChildren(t *testing.T) {
	def := &recordingRunner{name: "default"}
	a := &recordingRunner{name: "a"}
	b := &recordingRunner{name: "b"}
	rr := &RoutingRunner{
		Default:    def,
		ByAssignee: map[string]Runner{"a": a, "b": b},
	}
	if err := rr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !def.close || !a.close || !b.close {
		t.Errorf("close coverage: default=%v a=%v b=%v", def.close, a.close, b.close)
	}
}
