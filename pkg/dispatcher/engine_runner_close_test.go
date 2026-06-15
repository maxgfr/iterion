package dispatcher

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Close must run the bundle cleanup exactly once even under concurrent
// calls — the old nil-check-then-swap was a read-read-write race that
// let two callers both invoke RemoveAll on the extraction dir. Run with
// -race to also catch the field race the sync.Once removes.
func TestEngineRunnerCloseRunsCleanupOnce(t *testing.T) {
	var calls int32
	r := &EngineRunner{bundleClean: func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	}}

	const N = 16
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = r.Close()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("bundleClean ran %d times; want exactly 1", got)
	}
}
