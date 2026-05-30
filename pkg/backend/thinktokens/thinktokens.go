// Package thinktokens provides an approximate token count for extended-thinking
// (reasoning) text.
//
// The Anthropic Messages API does not report a separate count of thinking
// tokens — extended-thinking output is billed as part of output_tokens with no
// breakdown. To surface a per-node thinking-token figure we re-encode the
// accumulated thinking text ourselves with a real BPE tokenizer.
//
// The o200k_base encoding is OpenAI's, not Anthropic's (whose tokenizer is not
// public), so the figure is an APPROXIMATION — always presented with a "~"
// prefix in logs/UI and never claimed to be exact. It is applied uniformly to
// both the claw and claude_code backends so the numbers are comparable, and it
// is materially closer than a chars/4 heuristic.
//
// This is a leaf package (imports only the tokenizer) so it can be shared by
// both pkg/backend/model and pkg/backend/delegate without an import cycle.
package thinktokens

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

const charsPerTokenFallback = 4

var (
	codecOnce sync.Once
	codec     tokenizer.Codec
)

// Count returns an approximate token count for thinking text.
//
// It encodes the text with the o200k_base BPE tokenizer (vocab compiled into
// the binary — no network, deterministic). If the codec fails to initialise or
// to encode, it falls back to a chars/4 heuristic so callers always get a
// usable figure. Empty input returns 0.
func Count(text string) int {
	if text == "" {
		return 0
	}
	codecOnce.Do(func() {
		// Error deliberately swallowed: a nil codec triggers the fallback.
		codec, _ = tokenizer.Get(tokenizer.O200kBase)
	})
	if codec != nil {
		if n, err := codec.Count(text); err == nil {
			return n
		}
	}
	return len(text)/charsPerTokenFallback + 1
}
