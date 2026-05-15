package conductor

import (
	"context"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// state is the in-memory bookkeeping the conductor's actor goroutine
// owns. Outside callers must reach it through typed commands posted on
// Conductor.cmds — never read these maps directly.
type state struct {
	running       map[string]*runningEntry
	claimed       map[string]struct{}
	retryAttempts map[string]int
	retryTimers   map[string]*time.Timer
	slotsByState  map[string]int // running count per workflow state
}

func newState() *state {
	return &state{
		running:       map[string]*runningEntry{},
		claimed:       map[string]struct{}{},
		retryAttempts: map[string]int{},
		retryTimers:   map[string]*time.Timer{},
		slotsByState:  map[string]int{},
	}
}

// runningEntry tracks one in-flight dispatch. It outlives the actor's
// view of the goroutine: the goroutine writes LastEventAt via cmdEvent.
type runningEntry struct {
	IssueID       string
	Identifier    string
	RunID         string
	WorkflowState string
	WorkspacePath string
	StartedAt     time.Time
	LastEventAt   time.Time
	LastEventName string
	Attempt       int
	Cancel        context.CancelFunc

	// issueSnapshot is the tracker.Issue snapshot used to render
	// dispatch.vars. Kept so the conductor can render a fresh prompt
	// on retry without re-fetching from the tracker.
	issueSnapshot tracker.Issue
}

// Snapshot is the read-only view the dashboard consumes. Built on
// demand from inside the actor so callers always see a consistent
// snapshot of running/retries/slots.
type Snapshot struct {
	Name             string         `json:"name"`
	Tracker          string         `json:"tracker"`
	GeneratedAt      time.Time      `json:"generated_at"`
	PollingIntervalS float64        `json:"polling_interval_seconds"`
	StallTimeoutS    float64        `json:"stall_timeout_seconds"`
	Running          []RunningView  `json:"running"`
	Retries          []RetryView    `json:"retries"`
	Slots            SlotsView      `json:"slots"`
}

// RunningView is one row of the dashboard's "running" table.
type RunningView struct {
	IssueID       string    `json:"issue_id"`
	Identifier    string    `json:"identifier"`
	RunID         string    `json:"run_id"`
	WorkflowState string    `json:"workflow_state"`
	WorkspacePath string    `json:"workspace_path,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	LastEventAt   time.Time `json:"last_event_at"`
	LastEventName string    `json:"last_event_name,omitempty"`
	Attempt       int       `json:"attempt,omitempty"`
}

// RetryView is one row of the dashboard's "retries" table.
type RetryView struct {
	IssueID    string    `json:"issue_id"`
	Identifier string    `json:"identifier"`
	Attempt    int       `json:"attempt"`
	DueAt      time.Time `json:"due_at"`
	Error      string    `json:"error,omitempty"`
}

// SlotsView reports concurrency usage at the moment of capture.
type SlotsView struct {
	GlobalMax   int            `json:"global_max"`
	GlobalUsed  int            `json:"global_used"`
	PerStateMax map[string]int `json:"per_state_max,omitempty"`
	PerStateUsed map[string]int `json:"per_state_used,omitempty"`
}
