// Package generic decodes the bot-agnostic JSON shape iterion accepts
// on /api/webhooks/generic/{id}. Operators use it to wire CI runners,
// internal automation, or non-forge events into iterion without having
// to model the source in code.
//
// The wire schema is deliberately closed: a single Request struct with
// validated field bounds. We want a stable contract more than we want
// passthrough flexibility — every var an automated caller wants to
// stamp on the run is named explicitly. Operator-supplied LaunchVars on
// the Config OVERRIDE the request body (the operator is the
// security-critical knob; a malicious caller cannot escalate by
// renaming a var).
package generic

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Request is the validated JSON shape callers POST. All fields are
// optional individually; together at least Bot OR a Config-default
// must be set (the handler enforces that — at the schema layer we just
// validate shapes).
type Request struct {
	// Bot picks the bot to launch when the config has no default and no
	// single-bot scope. Names matching the bot registry; empty means
	// "use config default".
	Bot string `json:"bot"`

	// Vars is the per-run variable map merged into the launch. Each
	// key must match [A-Za-z_][A-Za-z0-9_]{0,63}; each value is capped
	// at 4 KiB; the map itself is capped at 256 entries. The Config's
	// LaunchVars overlay after this map (operator-pinned vars win) —
	// see the handler for the precedence rule.
	Vars map[string]string `json:"vars"`

	// IdempotencyKey, when non-empty, is the dedup token the handler
	// hashes into the delivery row's idempotency key. Empty → the
	// handler falls back to sha256(body), so an exact retransmission
	// still dedupes.
	IdempotencyKey string `json:"idempotency_key"`

	// RepoURL + RepoRef are the git pointers passed to the runner. Both
	// empty means "no clone" — the runner treats this as a workflow
	// that operates without a working tree (a deliberate use case for
	// dispatcher-style automation).
	RepoURL string `json:"repo_url"`
	RepoRef string `json:"repo_ref"`

	// ProjectPath is the audit + allowlist key. Empty disables the
	// ProjectAllowlist gate (a generic webhook without project context
	// is permitted; the operator who set the allowlist accepts that).
	ProjectPath string `json:"project_path"`
}

// MaxVars bounds the per-request variable map; ergonomic caps keep a
// rogue caller from exhausting the launch-time map allocation.
const (
	MaxVars         = 256
	MaxVarValueSize = 4096
)

var (
	varKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

	ErrTooManyVars      = errors.New("generic: too many vars (max 256)")
	ErrBadVarKey        = errors.New("generic: var key must match [A-Za-z_][A-Za-z0-9_]{0,63}")
	ErrVarValueTooLarge = errors.New("generic: var value exceeds 4 KiB")
)

// ParseRequest decodes + validates a generic webhook body. Anything
// the wire shape allows but the schema rejects (oversized vars,
// unsafe key) is a 400 in the handler — the caller MUST not be able
// to stamp arbitrary keys into the run env.
func ParseRequest(body []byte) (Request, error) {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return Request{}, fmt.Errorf("generic: decode request: %w", err)
	}
	if len(req.Vars) > MaxVars {
		return Request{}, ErrTooManyVars
	}
	for k, v := range req.Vars {
		if !varKeyRE.MatchString(k) {
			return Request{}, fmt.Errorf("%w: %q", ErrBadVarKey, k)
		}
		if len(v) > MaxVarValueSize {
			return Request{}, fmt.Errorf("%w: %q (%d bytes)", ErrVarValueTooLarge, k, len(v))
		}
	}
	return req, nil
}
