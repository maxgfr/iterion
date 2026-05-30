package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DailySpend is the per-(store, UTC-day) LLM spend ledger backing the
// daily spend cap. It is persisted at <root>/spend/<YYYY-MM-DD>.json.
//
// Accumulation is idempotent across resume/restart: RunsContributed maps
// each run ID to that run's LATEST cumulative cost (not a delta), and
// SpentUSD is always recomputed as the sum over RunsContributed. Re-
// recording a run that already contributed simply overwrites its entry
// with the newer cumulative, so a resumed/re-executed node cannot
// double-count the whole run.
type DailySpend struct {
	// Date is the UTC calendar day key (YYYY-MM-DD).
	Date string `json:"date"`
	// SpentUSD is the sum of RunsContributed — the day's total LLM spend.
	SpentUSD float64 `json:"spent_usd"`
	// RunsContributed maps run ID → that run's latest cumulative cost.
	RunsContributed map[string]float64 `json:"runs_contributed,omitempty"`
	// Override, when Active, suspends the cap for this day only. Cleared
	// automatically by the next day's fresh (absent) ledger.
	Override *SpendOverride `json:"override,omitempty"`
	// UpdatedAt is the wall-clock time of the last write.
	UpdatedAt time.Time `json:"updated_at"`
}

// SpendOverride records an operator's "override the cap for today"
// decision. It is intentionally audit-friendly: who, when, and why.
type SpendOverride struct {
	Active    bool      `json:"active"`
	GrantedAt time.Time `json:"granted_at"`
	GrantedBy string    `json:"granted_by,omitempty"`
	Note      string    `json:"note,omitempty"`
}

// spendDir is the directory holding per-day ledgers.
func (s *FilesystemRunStore) spendDir() string {
	return filepath.Join(s.root, "spend")
}

// spendJSONPath returns the on-disk path for a given day's ledger.
func (s *FilesystemRunStore) spendJSONPath(date string) (string, error) {
	if err := sanitizePathComponent("spend date", date); err != nil {
		return "", err
	}
	return filepath.Join(s.spendDir(), date+".json"), nil
}

// loadSpendRaw reads a day's ledger without locking. Returns a zero-
// valued ledger (not an error) when the file is absent — an empty day
// has simply spent nothing yet.
func (s *FilesystemRunStore) loadSpendRaw(date string) (*DailySpend, error) {
	path, err := s.spendJSONPath(date)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DailySpend{Date: date, RunsContributed: map[string]float64{}}, nil
		}
		return nil, fmt.Errorf("store: load spend %s: %w", date, err)
	}
	var ds DailySpend
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("store: decode spend %s: %w", date, err)
	}
	if ds.Date == "" {
		ds.Date = date
	}
	if ds.RunsContributed == nil {
		ds.RunsContributed = map[string]float64{}
	}
	return &ds, nil
}

// writeSpend persists a ledger atomically. Caller holds s.mu.
func (s *FilesystemRunStore) writeSpend(ds *DailySpend) error {
	if err := os.MkdirAll(s.spendDir(), dirPerm); err != nil {
		return fmt.Errorf("store: create spend dir: %w", err)
	}
	path, err := s.spendJSONPath(ds.Date)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return fmt.Errorf("store: encode spend %s: %w", ds.Date, err)
	}
	return WriteFileAtomic(path, data, filePerm)
}

// recomputeSpent sets SpentUSD to the sum of RunsContributed.
func recomputeSpent(ds *DailySpend) {
	var total float64
	for _, c := range ds.RunsContributed {
		total += c
	}
	ds.SpentUSD = total
}

// LoadDailySpend reads the ledger for a day. Never errors on absence.
func (s *FilesystemRunStore) LoadDailySpend(_ context.Context, date string) (*DailySpend, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSpendRaw(date)
}

// AddSpend records run runID's latest cumulative cost into the day's
// ledger and returns the updated ledger. Idempotent: passing the same
// (date, runID) with a higher cumulative overwrites the prior value;
// the day total is recomputed as the sum across all contributing runs.
func (s *FilesystemRunStore) AddSpend(_ context.Context, date, runID string, cumulativeRunCostUSD float64) (*DailySpend, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ds, err := s.loadSpendRaw(date)
	if err != nil {
		return nil, err
	}
	if ds.RunsContributed == nil {
		ds.RunsContributed = map[string]float64{}
	}
	// Monotonic per run: never let a stale lower cumulative shrink the
	// recorded contribution (out-of-order writes across branches).
	if prev, ok := ds.RunsContributed[runID]; !ok || cumulativeRunCostUSD > prev {
		ds.RunsContributed[runID] = cumulativeRunCostUSD
	}
	recomputeSpent(ds)
	ds.UpdatedAt = time.Now().UTC()
	if err := s.writeSpend(ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// SetSpendOverride sets (or clears) the override flag for a day and
// returns the updated ledger.
func (s *FilesystemRunStore) SetSpendOverride(_ context.Context, date string, ov *SpendOverride) (*DailySpend, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ds, err := s.loadSpendRaw(date)
	if err != nil {
		return nil, err
	}
	ds.Override = ov
	ds.UpdatedAt = time.Now().UTC()
	if err := s.writeSpend(ds); err != nil {
		return nil, err
	}
	return ds, nil
}
