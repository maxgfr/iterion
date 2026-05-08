// Package asymptote computes per-iteration quality scores from persisted
// runs and compares groups of runs (e.g. single-model vs alternated-model)
// to surface the asymptote thesis empirically.
//
// See docs/why-iterion.md for the thesis and the user-facing CLI command
// `iterion bench asymptote --single-runs ... --alternated-runs ... --output ...`.
package asymptote

import "time"

// IterationScore is one quality measurement at iteration N of a bounded
// loop in a single run. Iteration is 0-based (matches EventEdgeSelected
// data["iteration"]).
type IterationScore struct {
	RunID     string    `json:"run_id"`
	LoopName  string    `json:"loop_name"`
	Iteration int       `json:"iteration"`
	Score     float64   `json:"score"`     // canonical 0..1
	Approved  bool      `json:"approved"`  // true when score >= ApprovalThreshold
	NodeID    string    `json:"node_id"`   // judge node that produced the verdict
	RawValue  string    `json:"raw_value"` // the raw string before mapping (for debug)
	At        time.Time `json:"at"`
}

// RunSeries is the ordered list of IterationScore for a single run, sorted
// by iteration ascending. Always corresponds to one bounded loop (the one
// the judge node lives in).
type RunSeries struct {
	RunID    string           `json:"run_id"`
	LoopName string           `json:"loop_name"`
	JudgeID  string           `json:"judge_id"`
	Scores   []IterationScore `json:"scores"`
	// Status is the terminal Run.Status value at parse time, surfaced so
	// reports can flag truncated/aborted runs.
	Status string `json:"status"`
}

// GroupAggregate summarises a set of runs (e.g. all single-model runs) by
// iteration index: the per-iteration mean, the count of runs that reached
// that iteration, and the stderr.
type GroupAggregate struct {
	Label   string               `json:"label"` // "single-model" / "alternated"
	Runs    []RunSeries          `json:"runs"`
	PerIter []IterationAggregate `json:"per_iter"` // index = iteration number
	MaxIter int                  `json:"max_iter"`
}

// IterationAggregate is the aggregate score across a group at a given
// iteration index.
type IterationAggregate struct {
	Iteration int     `json:"iteration"`
	Count     int     `json:"count"`
	MeanScore float64 `json:"mean_score"`
	StdErr    float64 `json:"stderr"`
	PassRate  float64 `json:"pass_rate"` // fraction of runs approved at this iter
}

// Comparison is the side-by-side result rendered in the report.
type Comparison struct {
	Single     GroupAggregate `json:"single"`
	Alternated GroupAggregate `json:"alternated"`
	// MaxIter is the max of Single.MaxIter and Alternated.MaxIter, the
	// extent of the shared X-axis when graphing.
	MaxIter int `json:"max_iter"`
}

// ParseOptions tunes how a single run's events are interpreted.
type ParseOptions struct {
	// JudgeNodeID is the IR node ID whose EventNodeFinished payload carries
	// the verdict. Required.
	JudgeNodeID string
	// LoopName restricts parsing to one bounded loop. Optional — when empty
	// the parser picks the first loop name observed in EventEdgeSelected
	// that involves the judge node.
	LoopName string
	// JudgeField is the key inside EventNodeFinished.Data["output"] holding
	// the verdict. Default "approved".
	JudgeField string
	// ApprovalThreshold defines the score above which IterationScore.Approved
	// is true. Default 0.5 (so booleans land cleanly).
	ApprovalThreshold float64
}

// DefaultJudgeField is the conventional bool field iterion workflows use
// to mark a judge's verdict.
const DefaultJudgeField = "approved"

// DefaultApprovalThreshold is the score above which an iteration is
// considered "passed".
const DefaultApprovalThreshold = 0.5
