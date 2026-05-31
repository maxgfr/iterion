package main

import "sync"

// consentState is one participant's consent decision in one thread.
type consentState int

const (
	consentUnknown consentState = iota // never prompted / never answered
	consentGranted
	consentDeclined
)

// ConsentStore tracks, per (thread, user), whether the participant has
// agreed to have their messages sent to the LLM. Consent is scoped to a
// THREAD (root post id), never a whole channel: agreeing in one thread
// says nothing about another.
//
// It also remembers which threads are "active" (a participant mentioned
// @clarify-bot) and which users have already been shown the consent
// prompt, so the adapter re-prompts a newly-appearing participant
// exactly once.
//
// Safe for concurrent use. In-memory by design — consent is
// intentionally ephemeral; a restart re-asks, which is the privacy-safe
// default.
type ConsentStore struct {
	mu       sync.RWMutex
	active   map[string]bool                    // threadID → activated
	consent  map[string]map[string]consentState // threadID → userID → state
	prompted map[string]map[string]bool         // threadID → userID → prompted
}

// NewConsentStore returns an empty store.
func NewConsentStore() *ConsentStore {
	return &ConsentStore{
		active:   make(map[string]bool),
		consent:  make(map[string]map[string]consentState),
		prompted: make(map[string]map[string]bool),
	}
}

// Activate marks a thread as one @clarify-bot is facilitating.
func (c *ConsentStore) Activate(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active[threadID] = true
}

// IsActive reports whether the thread has been activated.
func (c *ConsentStore) IsActive(threadID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active[threadID]
}

// Set records a participant's consent decision for a thread.
func (c *ConsentStore) Set(threadID, userID string, granted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.consent[threadID]
	if m == nil {
		m = make(map[string]consentState)
		c.consent[threadID] = m
	}
	if granted {
		m[userID] = consentGranted
	} else {
		m[userID] = consentDeclined
	}
}

// HasConsented reports whether the participant has explicitly granted
// consent in this thread. Unknown and declined both return false — the
// privacy-safe default is exclusion.
func (c *ConsentStore) HasConsented(threadID, userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consent[threadID][userID] == consentGranted
}

// ConsentedSet returns the set of user ids that have granted consent in
// the thread, as a map suitable for Anonymizer.Transcript.
func (c *ConsentStore) ConsentedSet(threadID string) map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]bool)
	for uid, st := range c.consent[threadID] {
		if st == consentGranted {
			out[uid] = true
		}
	}
	return out
}

// NeedsPrompt reports whether userID should be shown the consent prompt
// in this thread: true when they have no recorded decision AND have not
// already been prompted. Calling it does not change state; pair it with
// MarkPrompted once the prompt is actually posted.
func (c *ConsentStore) NeedsPrompt(threadID, userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.consent[threadID][userID] != consentUnknown {
		return false
	}
	return !c.prompted[threadID][userID]
}

// MarkPrompted records that the consent prompt has been shown to userID
// in this thread, so NeedsPrompt won't ask again.
func (c *ConsentStore) MarkPrompted(threadID, userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.prompted[threadID]
	if m == nil {
		m = make(map[string]bool)
		c.prompted[threadID] = m
	}
	m[userID] = true
}
