package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/SocialGouv/claw-code-go/pkg/api"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
)

// ---------------------------------------------------------------------------
// Retry classifiers + the delegate-retry loop.
//
// Carved out of executor.go to keep the file's bulk focused on Execute
// flow control. Lives in the same package so the helpers stay private.
// ---------------------------------------------------------------------------

// isRetryable returns true if err is a transient LLM API error that should be
// retried. Recognises both iterion's local *APIError (used for stream-decoded
// errors) and claw-code-go's *clawapi.APIError (returned by provider HTTP
// clients on non-2xx responses, e.g. 429 / 5xx).
func isRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable
	}
	var clawErr *api.APIError
	if errors.As(err, &clawErr) {
		return clawErr.IsRetryable()
	}
	return false
}

// statusCodeOf extracts the HTTP status code from a recognised API error
// type, or 0 when the error is not an API error.
func statusCodeOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	var clawErr *api.APIError
	if errors.As(err, &clawErr) {
		return clawErr.StatusCode
	}
	return 0
}

// isDelegateRetryable determines whether a backend execution error is transient
// and worth retrying. Only signal-based kills (exit codes 128+, indicating
// OOM, SIGTERM, etc.) and I/O errors are retried. Permanent failures like
// exit 1 (application error), exit 2 (misuse), or exit 127 (command not
// found) are not retried.
//
// Typed-error fast paths come first: an *ErrTransient or *ErrRateLimited
// raised explicitly by a backend bypasses the legacy stderr-string
// heuristics, which are kept as a fallback for backends that haven't
// been migrated yet (and for SDK-internal errors we don't own).
func isDelegateRetryable(err error) bool {
	if err == nil {
		return false
	}
	var transient *delegate.ErrTransient
	if errors.As(err, &transient) {
		return true
	}
	var rateLimited *delegate.ErrRateLimited
	if errors.As(err, &rateLimited) {
		return true
	}
	msg := err.Error()
	// Subprocess killed by signal (OOM, timeout, etc.).
	if strings.Contains(msg, "signal:") {
		return true
	}
	// Exit status: only retry signal-based exits (128+). Lower exit codes
	// indicate permanent failures that retrying won't fix.
	if strings.Contains(msg, "exit status") {
		code := extractExitCode(msg)
		// exit 128+ means the process was killed by a signal (128+N).
		// These are typically transient (OOM killer, timeout, etc.).
		return code >= 128
	}
	// Process could not start (resource exhaustion).
	if strings.Contains(msg, "failed to start") {
		return true
	}
	// Stdout reading failure (broken pipe, etc.).
	if strings.Contains(msg, "reading stdout") {
		return true
	}
	// claude_code SDK fell silent for too long (we observed sessions
	// hanging in ep_poll without any propagated error). The runSession
	// watchdog aborts and surfaces this — retrying usually picks up
	// where the previous attempt left off because the resumed session
	// gets a fresh subprocess.
	if strings.Contains(msg, "session idle for") {
		return true
	}
	return false
}

// extractExitCode parses an exit code from an error message containing
// "exit status N". Returns -1 if no valid code is found.
func extractExitCode(msg string) int {
	const prefix = "exit status "
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return -1
	}
	rest := msg[idx+len(prefix):]
	// Parse the integer, stopping at first non-digit.
	n := 0
	found := false
	for _, c := range rest {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			found = true
		} else {
			break
		}
	}
	if !found {
		return -1
	}
	return n
}

// retryDelegateLoop retries a backend execution call with exponential backoff.
func (e *ClawExecutor) retryDelegateLoop(ctx context.Context, nodeID string, backendName string, fn func() (delegate.Result, error)) (delegate.Result, error) {
	maxAttempts := e.retry.maxAttempts()

	result, err := fn()
	for attempt := 1; err != nil && isDelegateRetryable(err) && attempt < maxAttempts; attempt++ {
		delay := e.retry.backoff(attempt - 1)

		e.logger.Warn("[%s#%d/%s] delegate retry %d/%d after error: %v (backoff %s)",
			nodeID, LoopIterationFromContext(ctx), backendName, attempt, maxAttempts-1, err, delay.Round(time.Millisecond))

		if e.hooks.OnDelegateRetry != nil {
			e.hooks.OnDelegateRetry(nodeID, DelegateInfo{
				BackendName: backendName,
				Attempt:     attempt,
				Error:       err,
				Delay:       delay,
			})
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return delegate.Result{}, ctx.Err()
		}

		result, err = fn()
	}
	return result, err
}
