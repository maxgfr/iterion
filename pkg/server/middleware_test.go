package server

import (
	"net/http/httptest"
	"testing"
)

// TestExtractBearerAcceptsTokenOnWSPaths verifies that the ?t=<jwt>
// query-param fallback for WebSocket clients works on both the
// file-event hub (/api/ws, no trailing slash) and the per-run streams
// (/api/ws/runs/<id>). A regression on this lets browser-driven WS
// authenticate while rejecting CLI/SDK clients on /api/ws, which
// cannot attach an Authorization header to the Upgrade request.
func TestExtractBearerAcceptsTokenOnWSPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want string
	}{
		{"/api/ws", "abc"},
		{"/api/ws/", "abc"},
		{"/api/ws/runs/foo", "abc"},
		{"/api/files/save", ""}, // ?t= must NOT leak onto regular HTTP routes
		{"/api/parse", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", tc.path+"?t=abc", nil)
			if got := extractBearer(req); got != tc.want {
				t.Fatalf("extractBearer(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
