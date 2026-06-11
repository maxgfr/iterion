package orgusage

import (
	"context"
	"sync"
	"time"
)

// MemoryCounter is the in-process Counter for tests and local mode.
// Keep its semantics in lock-step with MongoCounter.
type MemoryCounter struct {
	mu    sync.Mutex
	usage map[string]*memUsage // usageKey -> counters
}

type memUsage struct {
	runs          int
	costUSDMillis int64
	inputTokens   int64
	outputTokens  int64
}

func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{usage: make(map[string]*memUsage)}
}

func (c *MemoryCounter) get(tenantID string, when time.Time) *memUsage {
	key := usageKey(tenantID, when)
	u, ok := c.usage[key]
	if !ok {
		u = &memUsage{}
		c.usage[key] = u
	}
	return u
}

func (c *MemoryCounter) AllowRun(_ context.Context, tenantID string, when time.Time, maxRuns int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := c.get(tenantID, when)
	if maxRuns > 0 && u.runs+1 > maxRuns {
		return false, nil
	}
	u.runs++
	return true, nil
}

func (c *MemoryCounter) AddSpend(_ context.Context, tenantID string, when time.Time, costUSD float64, inputTokens, outputTokens int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := c.get(tenantID, when)
	u.costUSDMillis += costToMillis(costUSD)
	if inputTokens > 0 {
		u.inputTokens += inputTokens
	}
	if outputTokens > 0 {
		u.outputTokens += outputTokens
	}
	return nil
}

func (c *MemoryCounter) Usage(_ context.Context, tenantID string, when time.Time) (MonthlyUsage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := MonthlyUsage{Month: monthKey(when)}
	if u, ok := c.usage[usageKey(tenantID, when)]; ok {
		out.Runs = u.runs
		out.CostUSD = millisToCost(u.costUSDMillis)
		out.InputTokens = u.inputTokens
		out.OutputTokens = u.outputTokens
	}
	return out, nil
}
