package notify

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// resolveCallbackIP backs both the upfront vetURL check and the pinned
// delivery dialer: it must return the validated IP and reject private /
// metadata addresses unless allowPrivate is set.
func TestResolveCallbackIP(t *testing.T) {
	ctx := context.Background()
	pub := New(nil, time.Second)                          // allowPrivate=false
	priv := New(nil, time.Second, WithAllowPrivate(true)) // allowPrivate=true

	if ip, err := pub.resolveCallbackIP(ctx, "8.8.8.8"); err != nil || !ip.Equal(net.ParseIP("8.8.8.8")) {
		t.Fatalf("public numeric: ip=%v err=%v; want 8.8.8.8", ip, err)
	}
	if _, err := pub.resolveCallbackIP(ctx, "10.0.0.5"); err == nil {
		t.Fatal("private numeric must be rejected without allowPrivate")
	}
	if _, err := pub.resolveCallbackIP(ctx, "169.254.169.254"); err == nil {
		t.Fatal("cloud-metadata IP must be rejected")
	}
	if _, err := pub.resolveCallbackIP(ctx, "foo.svc.cluster.local"); err == nil {
		t.Fatal("cluster-internal alias must be rejected")
	}
	if ip, err := priv.resolveCallbackIP(ctx, "127.0.0.1"); err != nil || !ip.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("allowPrivate loopback: ip=%v err=%v; want 127.0.0.1", ip, err)
	}
}

// deliver must not auto-follow redirects: a 3xx to a fresh host would
// re-open the DNS-rebinding window (the new host is never re-validated).
func TestDeliverDoesNotFollowRedirects(t *testing.T) {
	var bHits int32
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&bHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamB.Close()

	var aHits int32
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&aHits, 1)
		http.Redirect(w, r, upstreamB.URL+"/next", http.StatusFound)
	}))
	defer upstreamA.Close()

	// allowPrivate so the loopback httptest targets are reachable.
	n := New(nil, 2*time.Second, WithAllowPrivate(true))
	n.deliver(context.Background(), upstreamA.URL+"/cb", CompletionPayload{RunID: "r"})

	if got := atomic.LoadInt32(&aHits); got != 1 {
		t.Fatalf("upstream A hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&bHits); got != 0 {
		t.Fatalf("redirect was followed: upstream B hits = %d, want 0", got)
	}
}
