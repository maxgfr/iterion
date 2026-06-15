package dispatcher

import "testing"

func TestCmdBufferSize(t *testing.T) {
	cases := []struct {
		maxConcurrent int
		want          int
	}{
		{0, 64},   // unset → floor
		{4, 64},   // low concurrency → floor
		{24, 64},  // 2*24+16=64 → still floor (boundary)
		{25, 66},  // just past the floor
		{64, 144}, // scales with concurrency
		{200, 416},
	}
	for _, c := range cases {
		if got := cmdBufferSize(c.maxConcurrent); got != c.want {
			t.Errorf("cmdBufferSize(%d) = %d; want %d", c.maxConcurrent, got, c.want)
		}
	}
}

// The buffer must always exceed MaxConcurrent so a full burst of
// cmdRunFinished (one per in-flight worker) never blocks on the send.
func TestCmdBufferSizeAlwaysExceedsConcurrency(t *testing.T) {
	for _, n := range []int{1, 10, 32, 64, 128, 500, 1000} {
		if got := cmdBufferSize(n); got <= n {
			t.Errorf("cmdBufferSize(%d) = %d; must exceed MaxConcurrent", n, got)
		}
	}
}
