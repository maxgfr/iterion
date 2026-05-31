package main

import "strings"

// RelevanceFilter decides, per new message, whether @clarify-bot should
// run the full facilitator. It is the cheap gate in front of the
// (more expensive) iterion run: the adapter calls it on every post in
// an active thread and only launches a run when ShouldRespond is true.
//
// Implementations must be safe for concurrent use and fast — they run
// on every message in every active thread.
type RelevanceFilter interface {
	// ShouldRespond reports whether latest (already anonymised, in the
	// context of transcript) is worth a facilitator run.
	ShouldRespond(transcript, latest string) bool
}

// heuristicFilter is the default, dependency-free RelevanceFilter. It
// is intentionally conservative-but-cheap: it never calls an LLM, so it
// is the safe out-of-the-box choice, but it is a coarse signal.
//
// PRODUCTION NOTE: the recommended setup replaces this with an
// LLM-backed filter (a cheap/fast model — e.g. a small claw model)
// implementing the same interface, which judges genuine ambiguity far
// better than keyword heuristics. This default is documented as such in
// the README so its limits are explicit (no silent cap): it responds to
// questions and to messages that signal confusion, and stays quiet
// otherwise.
type heuristicFilter struct {
	minLen int
}

func newHeuristicFilter() *heuristicFilter { return &heuristicFilter{minLen: 8} }

// confusionMarkers are substrings that suggest a participant is unsure
// or that a clarification would help. Lower-cased match.
var confusionMarkers = []string{
	"?", "what do you mean", "not sure", "unclear", "confused",
	"don't understand", "dont understand", "which one", "wdym",
	"can you clarify", "what's the", "whats the", "i thought",
}

func (h *heuristicFilter) ShouldRespond(_ /*transcript*/, latest string) bool {
	l := strings.ToLower(strings.TrimSpace(latest))
	if len(l) < h.minLen {
		return false // too short to carry real ambiguity
	}
	for _, m := range confusionMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
