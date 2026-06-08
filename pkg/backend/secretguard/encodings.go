package secretguard

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"
)

// encodingsOf returns the distinct textual representations a secret
// value can take on the wire or in a log. This is the heart of the
// "detect base64 / other formats" requirement: rather than guessing
// whether an arbitrary high-entropy blob is a secret, we precompute
// the encodings of the secrets we KNOW and match those literally.
//
// The raw value is always first; the remaining forms are the common
// ways a value gets reshaped before it lands in a request body, a
// JSON event, or a URL:
//   - base64 std / url, with and without padding
//   - hex, lower and upper case
//   - URL query / path percent-encoding
//   - JSON string escaping (the body between the quotes)
//
// Duplicates (an encoding equal to the raw value, or to a prior
// encoding) are dropped so the matcher stays minimal.
func encodingsOf(v string) []string {
	if v == "" {
		return nil
	}
	out := make([]string, 0, 12)
	seen := make(map[string]struct{}, 12)
	add := func(s string) {
		if s == "" {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	add(v) // raw

	b := []byte(v)
	add(base64.StdEncoding.EncodeToString(b))
	add(base64.RawStdEncoding.EncodeToString(b))
	add(base64.URLEncoding.EncodeToString(b))
	add(base64.RawURLEncoding.EncodeToString(b))

	hl := hex.EncodeToString(b)
	add(hl)
	add(strings.ToUpper(hl))

	add(url.QueryEscape(v))
	add(url.PathEscape(v))

	// JSON string escaping: marshal then strip the surrounding quotes,
	// leaving the escaped body (e.g. embedded `\"` / `\n` / `\\`).
	if j, err := json.Marshal(v); err == nil && len(j) >= 2 {
		add(string(j[1 : len(j)-1]))
	}

	return out
}

// tryDecode attempts to interpret tok as base64 (std/url, padded or
// not) or hex, returning the decoded string when it yields mostly
// printable text. It is used by the recursive-decode heuristic to
// peel one encoding layer off an UNKNOWN blob and re-scan the inner
// bytes for token shapes (e.g. an AKIA key wrapped in base64).
//
// An empty return means "not a useful decode" — the caller leaves the
// token untouched.
func tryDecode(tok string) string {
	if len(tok) < 12 {
		return ""
	}
	decoders := []func(string) ([]byte, error){
		func(s string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(s) },
		func(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) },
		func(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) },
		func(s string) ([]byte, error) { return base64.URLEncoding.DecodeString(s) },
		hex.DecodeString,
	}
	for _, dec := range decoders {
		raw, err := dec(tok)
		if err != nil || len(raw) < 8 {
			continue
		}
		if !mostlyPrintable(raw) {
			continue
		}
		return string(raw)
	}
	return ""
}

// mostlyPrintable reports whether b is valid UTF-8 made up mostly of
// printable characters — the signal that a decode produced text (a
// nested token) rather than binary garbage.
func mostlyPrintable(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	printable, total := 0, 0
	for _, r := range string(b) {
		total++
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != utf8.RuneError) {
			printable++
		}
	}
	if total == 0 {
		return false
	}
	return float64(printable)/float64(total) >= 0.85
}
