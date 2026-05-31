package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SignatureHeader is the HTTP header carrying the HMAC of a webhook
// body. Receivers read it and recompute the MAC over the raw body to
// authenticate the sender.
const SignatureHeader = "X-Iterion-Signature"

// signaturePrefix tags the algorithm in the header value, mirroring the
// GitHub/Stripe convention (e.g. "sha256=<hex>"). Keeping the algorithm
// in-band lets the scheme evolve without a second header.
const signaturePrefix = "sha256="

// Sign returns the header value authenticating body under secret:
// "sha256=" followed by the lowercase-hex HMAC-SHA256. An empty secret
// yields an empty string — callers treat that as "signing disabled" and
// send no signature header.
//
// This is the shared primitive for BOTH directions of iterion's webhook
// auth: the completion notifier signs outbound payloads with it, and any
// future native inbound webhook (POST → trigger a run) would verify
// incoming requests with Verify below. Keep it dependency-free.
func Sign(secret string, body []byte) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether header is a valid signature for body under
// secret. The comparison is constant-time to avoid leaking the expected
// MAC through timing. A mismatched length, missing prefix, or non-hex
// header all return false.
//
// When secret is empty, Verify returns false for every input — an
// unconfigured receiver must reject signed and unsigned requests alike
// rather than silently accept everything. Callers that want to allow
// unauthenticated requests must branch on "secret configured?" before
// calling Verify, making that choice explicit at the call site.
func Verify(secret string, body []byte, header string) bool {
	if secret == "" {
		return false
	}
	want := Sign(secret, body)
	// hmac.Equal is constant-time over equal-length inputs and
	// length-safe otherwise; compare the full "sha256=<hex>" strings so
	// a wrong prefix also fails.
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(header)))
}
