package main

import "sync"

// consentState is one participant's consent lifecycle in one thread.
//
// The states form a strict forward machine:
//
//	consentUnknown  --MarkPrompted--> consentPending
//	consentPending  --Set(true)-----> consentGranted
//	consentPending  --Set(false)----> consentDeclined
//
// "Has this user been prompted yet?" is therefore not a separate flag —
// it is simply "state != consentUnknown". Only consentUnknown needs a
// prompt; every later state means the prompt has already been shown.
type consentState int

const (
	consentUnknown  consentState = iota // never prompted / never answered
	consentPending                      // prompt shown, awaiting the user's click
	consentGranted                      // user agreed
	consentDeclined                     // user refused
)

// ConsentStore tracks, per (thread, user), where a participant is in the
// consent lifecycle. Consent is scoped to a THREAD (root post id), never
// a whole channel: agreeing in one thread says nothing about another.
//
// It also remembers which threads are "active" (a participant mentioned
// @clarify-bot).
//
// Safe for concurrent use. In-memory by design — consent is
// intentionally ephemeral; a restart re-asks, which is the privacy-safe
// default.
type ConsentStore struct {
	mu      sync.RWMutex
	active  map[string]bool                    // threadID → activated
	consent map[string]map[string]consentState // threadID → userID → state
}

// NewConsentStore returns an empty store.
func NewConsentStore() *ConsentStore {
	return &ConsentStore{
		active:  make(map[string]bool),
		consent: make(map[string]map[string]consentState),
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

// setState writes a participant's state, allocating the inner map lazily.
// Caller holds c.mu.
func (c *ConsentStore) setState(threadID, userID string, st consentState) {
	m := c.consent[threadID]
	if m == nil {
		m = make(map[string]consentState)
		c.consent[threadID] = m
	}
	m[userID] = st
}

// Set records a participant's consent decision for a thread.
func (c *ConsentStore) Set(threadID, userID string, granted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if granted {
		c.setState(threadID, userID, consentGranted)
	} else {
		c.setState(threadID, userID, consentDeclined)
	}
}

// HasConsented reports whether the participant has explicitly granted
// consent in this thread. Every non-granted state (unknown, pending,
// declined) returns false — the privacy-safe default is exclusion.
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
// in this thread: true only when they are in the initial consentUnknown
// state (never prompted, never answered). Calling it does not change
// state; pair it with MarkPrompted once the prompt is actually posted.
func (c *ConsentStore) NeedsPrompt(threadID, userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consent[threadID][userID] == consentUnknown
}

// MarkPrompted advances a participant from consentUnknown to
// consentPending, so NeedsPrompt won't ask again. A no-op once the user
// has already been prompted or has answered.
func (c *ConsentStore) MarkPrompted(threadID, userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.consent[threadID][userID] == consentUnknown {
		c.setState(threadID, userID, consentPending)
	}
}
