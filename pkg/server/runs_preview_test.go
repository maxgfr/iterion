package server

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/store"
)

func TestIsPublicUnicast(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false},
		{"169.254.1.2", false},
		{"100.100.100.200", false}, // alibaba metadata
		{"::1", false},
		{"fe80::1", false},
		{"fd00::1", false},
		{"2001:4860:4860::8888", true},
		{"0.0.0.0", false},
		{"224.0.0.1", false}, // multicast
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("ParseIP(%q) failed", tc.ip)
			}
			if got := isPublicUnicast(ip); got != tc.want {
				t.Fatalf("isPublicUnicast(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestResolvePreviewHost_NumericIP(t *testing.T) {
	cases := []struct {
		name      string
		host      string
		cloudMode bool
		wantErr   bool
	}{
		{"public ip in cloud", "8.8.8.8", true, false},
		{"public ip in local", "8.8.8.8", false, false},
		{"loopback in cloud", "127.0.0.1", true, true},
		{"loopback in local", "127.0.0.1", false, false},
		{"private in cloud", "192.168.1.1", true, true},
		{"private in local", "192.168.1.1", false, false},
		{"metadata in cloud", "169.254.169.254", true, true},
		{"metadata in local", "169.254.169.254", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip, err := resolvePreviewHost(context.Background(), tc.host, tc.cloudMode)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s in cloud=%v, got ip=%v", tc.host, tc.cloudMode, ip)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ip == nil || ip.String() != tc.host {
				t.Fatalf("got ip=%v, want %s", ip, tc.host)
			}
		})
	}
}

func TestResolvePreviewHost_ClusterInternalNames(t *testing.T) {
	rejected := []string{
		"kubernetes.default.svc.cluster.local",
		"my-service.my-ns.svc.cluster.local",
		"metadata.google.internal",
		"foo.svc",
	}
	for _, h := range rejected {
		t.Run(h, func(t *testing.T) {
			_, err := resolvePreviewHost(context.Background(), h, true)
			if err == nil {
				t.Fatalf("expected refusal of %s in cloud mode", h)
			}
			if !strings.Contains(err.Error(), "cluster-internal") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolvePreviewHost_EmptyHost(t *testing.T) {
	if _, err := resolvePreviewHost(context.Background(), "", false); err == nil {
		t.Fatal("expected error for empty host")
	}
	if _, err := resolvePreviewHost(context.Background(), "", true); err == nil {
		t.Fatal("expected error for empty host")
	}
}

// TestHandlePreviewProxy_RejectsPrivateTargetStrict exercises the SSRF guard
// end-to-end through the HTTP handler in strict (cloud) mode: private,
// loopback and cloud-metadata targets must be refused with 403 before any
// connection is attempted, malformed schemes/hosts with 400, and an unknown
// run id with 404 (relay prevention).
func TestHandlePreviewProxy_RejectsPrivateTargetStrict(t *testing.T) {
	srv, hs := newTestServer(t)
	srv.cfg.Mode = "cloud" // force strict SSRF validation regardless of bind
	seedRun(t, srv, "run-ssrf", "wf", store.RunStatusRunning)

	get := func(t *testing.T, runID, target string) int {
		t.Helper()
		u := hs.URL + "/api/runs/" + runID + "/preview?target=" + url.QueryEscape(target)
		resp, err := http.Get(u) // #nosec G107 G704 — test URL targets the local httptest server
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	t.Run("metadata endpoint rejected (403)", func(t *testing.T) {
		if got := get(t, "run-ssrf", "http://169.254.169.254/latest/meta-data/"); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})
	t.Run("private rfc1918 rejected (403)", func(t *testing.T) {
		if got := get(t, "run-ssrf", "http://192.168.0.1/"); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})
	t.Run("loopback rejected (403)", func(t *testing.T) {
		if got := get(t, "run-ssrf", "http://127.0.0.1:9/"); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})
	t.Run("non-http scheme rejected (400)", func(t *testing.T) {
		if got := get(t, "run-ssrf", "file:///etc/passwd"); got != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", got)
		}
	})
	t.Run("missing target rejected (400)", func(t *testing.T) {
		// Empty target is indistinguishable from an omitted one: the handler's
		// `target == ""` check rejects both with 400.
		if got := get(t, "run-ssrf", ""); got != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", got)
		}
	})
	t.Run("unknown run rejected before dial (404)", func(t *testing.T) {
		if got := get(t, "does-not-exist", "http://8.8.8.8/"); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})
}
