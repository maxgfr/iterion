package netproxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// SecretRewriter is the proxy's view of the secret guard, used by the
// TLS-inspection mode to rewrite plaintext requests (Layer 2). It is a
// structural interface so netproxy stays decoupled from
// pkg/backend/secretguard (which implements it).
type SecretRewriter interface {
	// MaterializeForHost swaps secret placeholders for real values, but
	// only for secrets scoped to (or unrestricted toward) host.
	MaterializeForHost(s, host string) string
	// ExfiltratesTo reports whether s carries a real secret value bound
	// for a host that secret is NOT scoped to (a blockable exfiltration).
	ExfiltratesTo(s, host string) bool
}

// inspectConfig holds the per-proxy TLS-inspection state. Nil disables
// inspection (the proxy stays a transparent CONNECT tunnel).
type inspectConfig struct {
	ca       *EphemeralCA
	rewriter SecretRewriter
	upstream http.RoundTripper
}

// handleConnectInspect terminates TLS on the hijacked client connection,
// rewrites each plaintext request (placeholder→secret for approved
// hosts; block on exfiltration to unapproved hosts), and forwards to the
// real upstream over a freshly-verified TLS connection. hostPort is the
// CONNECT target (host:port); the bare hostname drives leaf minting,
// substitution scoping, and DLP.
func (p *Proxy) handleConnectInspect(hostPort string, clientConn net.Conn, bufrw *bufio.ReadWriter) {
	defer clientConn.Close()

	hostname := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		hostname = h
	}

	// Read through the buffered reader (it already wraps clientConn and
	// may hold the ClientHello); write raw to clientConn.
	cc := &bufferedConn{Conn: clientConn, r: bufrw.Reader}
	tlsClient := tls.Server(cc, &tls.Config{
		GetCertificate: p.inspect.ca.GetCertificate,
		// Offer only HTTP/1.1 so we never have to demux HTTP/2 frames —
		// clients fall back transparently.
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
	})
	if err := tlsClient.Handshake(); err != nil {
		return
	}
	defer tlsClient.Close()

	reader := bufio.NewReader(tlsClient)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return // client closed, or malformed — done with this conn
		}
		keepAlive := p.serveInspectedRequest(tlsClient, req, hostname, hostPort)
		if !keepAlive {
			return
		}
	}
}

// serveInspectedRequest rewrites and forwards one request, writing the
// upstream response back to the client. Returns whether the connection
// may be reused for another request.
func (p *Proxy) serveInspectedRequest(client net.Conn, req *http.Request, hostname, hostPort string) bool {
	body, _ := io.ReadAll(io.LimitReader(req.Body, 64<<20))
	_ = req.Body.Close()

	rw := p.inspect.rewriter
	if rw != nil {
		// DLP: a real secret value leaving toward an unapproved host is
		// blocked outright (defeats domain-fronting the allowlist can't
		// see).
		scan := inspectScanText(req, body)
		if rw.ExfiltratesTo(scan, hostname) {
			writeSimpleResponse(client, http.StatusForbidden, "blocked by sandbox secret policy")
			if p.onBlocked != nil {
				p.onBlocked(hostname, "secret exfiltration blocked")
			}
			return false
		}
		// Substitute placeholders for real values, scoped to this host.
		body = []byte(rw.MaterializeForHost(string(body), hostname))
		for k, vals := range req.Header {
			for i, v := range vals {
				req.Header[k][i] = rw.MaterializeForHost(v, hostname)
			}
		}
	}

	// Rebuild the request for the upstream RoundTrip.
	outURL := *req.URL
	outURL.Scheme = "https"
	if outURL.Host == "" {
		outURL.Host = req.Host
	}
	if outURL.Host == "" {
		outURL.Host = hostPort
	}
	req.URL = &outURL
	req.RequestURI = ""
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))

	resp, err := p.inspect.upstream.RoundTrip(req)
	if err != nil {
		writeSimpleResponse(client, http.StatusBadGateway, "upstream error: "+err.Error())
		return false
	}
	defer resp.Body.Close()

	// Force connection-close framing on the response we hand back to the
	// client. Streaming LLM/SSE endpoints (and any HTTP/1.1 response with
	// no Content-Length and no chunked transfer-encoding) are
	// *close-delimited*: the body ends when the server closes the socket.
	// If we keep the inspected client connection alive after such a
	// response, the in-container HTTP/1.1 client blocks forever waiting for
	// more bytes — observed as a hard hang on every sandboxed `claw` LLM
	// call once Layer-2 TLS inspection is active. Emitting `Connection:
	// close` and tearing the conn down after the body lets the client
	// detect EOF deterministically. resp.Write copies the body straight to
	// the raw *tls.Conn (no buffering), so streamed tokens still flush as
	// they arrive; we trade HTTP keep-alive reuse for correctness, and the
	// client simply opens a fresh connection for its next request.
	resp.Close = true
	if err := resp.Write(client); err != nil {
		return false
	}
	return false
}

// inspectScanText assembles the request surface a secret could leak
// through: the URL, every header value, and the body.
func inspectScanText(req *http.Request, body []byte) string {
	var b strings.Builder
	b.Grow(len(body) + 256)
	b.WriteString(req.URL.String())
	b.WriteByte('\n')
	for k, vals := range req.Header {
		for _, v := range vals {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	b.Write(body)
	return b.String()
}

// writeSimpleResponse writes a minimal HTTP/1.1 response to a raw conn.
func writeSimpleResponse(conn net.Conn, code int, msg string) {
	resp := &http.Response{
		StatusCode: code,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type":     {"text/plain; charset=utf-8"},
			"X-Iterion-Reason": {"sandbox secret policy"},
		},
		Body:          io.NopCloser(strings.NewReader(msg + "\n")),
		ContentLength: int64(len(msg) + 1),
	}
	_ = resp.Write(conn)
}

// bufferedConn is a net.Conn whose Read is served from a buffered reader
// (which already wraps the underlying conn), while Write/Close/etc.
// delegate to the underlying conn. Used so TLS termination consumes any
// bytes the HTTP server already buffered past the CONNECT line.
type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
