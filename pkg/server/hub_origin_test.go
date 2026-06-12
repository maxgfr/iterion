package server

import (
	"net/http/httptest"
	"testing"
)

// TestSameWSOrigin locks the same-origin WS allow rule that fixes the cloud
// studio (the SPA dialing wss:// on the host that served it): the Origin
// header's host must match the request Host, scheme-agnostic + case-insensitive.
func TestSameWSOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"cloud same-origin", "https://iterion.ovh.fabrique.social.gouv.fr", "iterion.ovh.fabrique.social.gouv.fr", true},
		{"local same-origin", "http://localhost:4891", "localhost:4891", true},
		{"case-insensitive host", "https://Iterion.Example.com", "iterion.example.com", true},
		{"cross-site drive-by", "https://evil.example", "iterion.ovh.fabrique.social.gouv.fr", false},
		{"different port", "http://localhost:5173", "localhost:4891", false},
		{"empty origin", "", "iterion.ovh.fabrique.social.gouv.fr", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/ws/runs/x", nil)
			r.Host = c.host
			if got := sameWSOrigin(c.origin, r); got != c.want {
				t.Errorf("sameWSOrigin(%q, host=%q) = %v; want %v", c.origin, c.host, got, c.want)
			}
		})
	}
}
