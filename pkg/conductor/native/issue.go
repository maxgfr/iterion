package native

import "time"

// Issue is the native tracker's source-of-truth issue record. The
// conductor consumes a normalized view via tracker.Issue (see
// pkg/conductor/tracker/native.go for the conversion).
type Issue struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Body      string         `json:"body,omitempty"`
	State     string         `json:"state"`
	Labels    []string       `json:"labels,omitempty"`
	Priority  int            `json:"priority,omitempty"`
	Assignee  string         `json:"assignee,omitempty"`
	Blockers  []string       `json:"blockers,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	Claim     string         `json:"claim,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
