package cli

import (
	"fmt"
	"strings"
)

// kvOpts controls how parseKVPairs splits and validates a "key=value" flag.
// Each caller in the cli package wraps it with the exact error template,
// key-trim policy, and value conversion the caller's flag historically
// used — keeping their error messages byte-identical while sharing the
// scan loop.
type kvOpts[V any] struct {
	// errFmt is used as fmt.Errorf(errFmt, pair). It must contain exactly
	// one %q verb and embed the flag name (e.g. "invalid --var format %q
	// (expected key=value)").
	errFmt string
	// trimKey strips surrounding whitespace from the key when true. The
	// trim runs AFTER any empty-key check, so the check decides whether
	// the raw key is empty (matches the historical `eq <= 0` guard).
	trimKey bool
	// trimVal strips surrounding whitespace from the value when true.
	trimVal bool
	// requireRawKey rejects pairs whose raw (pre-trim) key is empty
	// (i.e. "=v"). Matches the historical `eq <= 0` guard.
	requireRawKey bool
	// requireTrimmedKey rejects pairs whose trimmed key is empty
	// (i.e. "=v" AND " =v"). Stricter than requireRawKey; matches the
	// historical `strings.TrimSpace(k) == ""` guard.
	requireTrimmedKey bool
	// conv converts the raw value string to V. May be nil when V == string,
	// in which case the value is stored as-is.
	conv func(string) V
}

// parseKVPairs splits each "k=v" element of pairs at the first '='.
// On missing '=' it returns fmt.Errorf(opts.errFmt, pair); on an empty
// key (raw or trimmed, per opts) it returns the same error. Allocates
// an empty (non-nil) map even on empty input — preserves the historical
// contract of every caller that allocates up-front. Callers that prefer
// nil-for-empty can short-circuit before calling.
//
// The check / trim order is:
//  1. cut at first '='
//  2. requireRawKey check (against the raw, pre-trim key)
//  3. requireTrimmedKey check (against TrimSpace(rawKey)) — this check
//     is independent of trimKey/trimVal so callers can validate against
//     a trimmed key without mutating the stored key.
//  4. trimKey / trimVal storage transforms
func parseKVPairs[V any](pairs []string, opts kvOpts[V]) (map[string]V, error) {
	out := make(map[string]V, len(pairs))
	for _, f := range pairs {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf(opts.errFmt, f)
		}
		if opts.requireRawKey && k == "" {
			return nil, fmt.Errorf(opts.errFmt, f)
		}
		if opts.requireTrimmedKey && strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf(opts.errFmt, f)
		}
		if opts.trimKey {
			k = strings.TrimSpace(k)
		}
		if opts.trimVal {
			v = strings.TrimSpace(v)
		}
		if opts.conv != nil {
			out[k] = opts.conv(v)
		} else {
			// V is constrained to be string (see callers) when conv is nil.
			out[k] = any(v).(V)
		}
	}
	return out, nil
}
