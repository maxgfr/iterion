package delegate

import "testing"

// TestIsTransientAPIErrorResult guards the overload/5xx guard added after a
// test-coverage dogfood saw the claude CLI return "API Error: 529 Overloaded"
// as a "successful" result text, which poisoned the node output and the
// session that inherited it. Transient classes must retry; client/auth errors
// and genuine assistant answers must not.
func TestIsTransientAPIErrorResult(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Transient — must retry.
		{"529 overloaded", "API Error: 529 Overloaded", true},
		{"529 json body", `API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, true},
		{"500 internal", "API Error: 500 Internal Server Error", true},
		{"503 unavailable", "API Error: 503 Service Unavailable", true},
		{"502 bad gateway", "API Error: 502 Bad Gateway", true},
		{"504 gateway timeout", "API Error: 504 Gateway Timeout", true},
		{"429 rate limited", "API Error: 429 Too Many Requests", true},
		{"no-colon overloaded", "API Error 529 Overloaded", true},
		{"leading/trailing space", "  API Error: 503 Service Unavailable\n", true},
		{"no code but connectivity marker", "API Error: Connection error.", true},
		{"case-insensitive prefix", "api error: 500 boom", true},

		// Non-transient client/auth errors — must surface, not loop.
		{"400 bad request", "API Error: 400 Bad Request", false},
		{"401 unauthorized", "API Error: 401 Unauthorized", false},
		{"403 forbidden", "API Error: 403 Forbidden", false},
		{"404 not found", "API Error: 404 Not Found", false},
		{"422 unprocessable", "API Error: 422 Unprocessable Entity", false},

		// Genuine assistant output — must never be mistaken for an error.
		{"empty", "", false},
		{"normal answer", "Here is the test plan: add unit tests for pkg/log.", false},
		{"discusses api error mid-text", "The handler should retry when it sees an API Error: 529 from upstream, so add a test for that path.", false},
		{"long text starting with api error word", "API errors are common; this suite asserts the client retries 5xx responses and surfaces 4xx ones, covering the full matrix of " + longPad(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientAPIErrorResult(tt.in); got != tt.want {
				t.Errorf("isTransientAPIErrorResult(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// longPad returns filler pushing a string past the 400-char brevity guard so
// the "long text" case exercises the length cutoff.
func longPad() string {
	s := ""
	for i := 0; i < 400; i++ {
		s += "x"
	}
	return s
}
