package dispatcher

import (
	"errors"
	"testing"
	"time"
)

// TestIsSandboxSetupError pins the classifier surface: every error
// message the sandbox driver / engine emits when the runtime never
// got to run a node must match, while genuine in-node failures (LLM
// errors, schema violations, http2 timeouts the recovery dispatch
// already handles) must miss so they keep the default exponential
// backoff. Match too tightly and a stress-induced postCreate failure
// re-spawns containers on a 10s exponential, piling docker churn on
// a host that needs minutes to recover.
func TestIsSandboxSetupError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"sandbox postCreate SIGKILL",
			errors.New(`sandbox start: runtime: sandbox: start: docker: postCreate failed: postCreateCommand exited 137:`),
			true,
		},
		{
			"sandbox postCreate exit -1",
			errors.New(`sandbox start: runtime: sandbox: start: docker: postCreate failed: postCreateCommand exited -1`),
			true,
		},
		{
			"buildkit image pull",
			errors.New(`sandbox: image pull failed: connection refused`),
			true,
		},
		{
			"container start refused",
			errors.New(`docker: container start: cannot allocate memory`),
			true,
		},
		{
			"broken claude binary (EFORMAT on first node — native:c6d93a2a)",
			errors.New(`node "reviewer_claude" execution failed: backend "claude_code" failed: delegate: claude-code failed: exec /usr/bin/claude: exec format error`),
			true,
		},
		{
			"http2 transient (handled by recovery dispatch, not sandbox path)",
			errors.New(`model: node "reviewer_gpt": http2: timeout awaiting response headers`),
			false,
		},
		{
			"plain execution failure",
			errors.New(`node "act" execution failed: schema validation error`),
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isSandboxSetupError(c.err)
			if got != c.want {
				t.Errorf("isSandboxSetupError(%q) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// TestSandboxBackoff covers the schedule + park transition. The
// dispatcher walks attempts 1..N with the explicit per-step delays
// from sandboxBackoffSchedule; once N is exceeded the entry is
// "parked" with sandboxParkDelay so the operator has space to fix
// the host without burning model spend on a postCreate that won't
// succeed in milliseconds.
func TestSandboxBackoff(t *testing.T) {
	cases := []struct {
		attempt    int
		wantDelay  time.Duration
		wantParked bool
	}{
		{0, time.Second, false}, // continuation
		{1, sandboxBackoffSchedule[0], false},
		{2, sandboxBackoffSchedule[1], false},
		{3, sandboxBackoffSchedule[2], false},
		{4, sandboxParkDelay, true},
		{10, sandboxParkDelay, true},
	}
	for _, c := range cases {
		d, parked := sandboxBackoff(c.attempt)
		if d != c.wantDelay || parked != c.wantParked {
			t.Errorf("sandboxBackoff(%d) = (%s, parked=%v), want (%s, parked=%v)",
				c.attempt, d, parked, c.wantDelay, c.wantParked)
		}
	}
}
