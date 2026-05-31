package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Message is one chat post the adapter has observed in a thread.
type Message struct {
	// UserID is the platform-stable author id (Mattermost user id).
	// Never leaves the adapter — it is the de-anonymisation key.
	UserID string
	// Text is the raw message content.
	Text string
	// CreateAtMillis orders messages chronologically within a thread.
	CreateAtMillis int64
}

// Anonymizer assigns stable, opaque pseudonyms ("User A", "User B", …)
// to real user ids WITHIN a single thread, and renders a transcript
// that contains ONLY consenting participants' messages.
//
// The pseudonym map is the de-anonymisation table; it is private to
// this process and is never serialised into a transcript, a callback
// token, or anything sent to the LLM. Pseudonyms are stable for the
// lifetime of the Anonymizer instance (one per thread): the same user
// id always maps to the same label, so the LLM can follow who-said-what
// across turns without ever seeing an identity.
//
// Safe for concurrent use.
type Anonymizer struct {
	mu     sync.Mutex
	labels map[string]string // userID → "User A"
	order  []string          // assignment order, for deterministic labelling
}

// NewAnonymizer returns an empty Anonymizer for one thread.
func NewAnonymizer() *Anonymizer {
	return &Anonymizer{labels: make(map[string]string)}
}

// Label returns the stable pseudonym for userID, assigning the next
// one ("User A", "User B", …) on first sight.
func (a *Anonymizer) Label(userID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.labelLocked(userID)
}

func (a *Anonymizer) labelLocked(userID string) string {
	if l, ok := a.labels[userID]; ok {
		return l
	}
	l := pseudonym(len(a.order))
	a.labels[userID] = l
	a.order = append(a.order, userID)
	return l
}

// Transcript renders msgs into a chronological, anonymised transcript
// string, INCLUDING ONLY messages whose author is in consented. A
// message from a non-consenting (or unknown) author is dropped entirely
// — neither its content nor a placeholder appears, so the LLM never
// learns a non-consenting participant even spoke. Pseudonyms are
// assigned (and reused) only for consenting authors.
//
// msgs need not be pre-sorted; Transcript orders by CreateAtMillis
// (stable on ties to preserve input order).
func (a *Anonymizer) Transcript(msgs []Message, consented map[string]bool) string {
	ordered := make([]Message, len(msgs))
	copy(ordered, msgs)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreateAtMillis < ordered[j].CreateAtMillis
	})

	a.mu.Lock()
	defer a.mu.Unlock()

	var b strings.Builder
	for _, m := range ordered {
		if !consented[m.UserID] {
			continue // excluded entirely — no content, no placeholder
		}
		label := a.labelLocked(m.UserID)
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", label, text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// pseudonym maps a 0-based index to "User A", "User B", … "User Z",
// "User AA", "User AB", … (spreadsheet-column style) so the scheme
// never runs out.
func pseudonym(i int) string {
	var sb strings.Builder
	i++ // 1-based for the modulo math
	for i > 0 {
		i--
		sb.WriteByte(byte('A' + i%26))
		i /= 26
	}
	// reverse
	s := []byte(sb.String())
	for l, r := 0, len(s)-1; l < r; l, r = l+1, r-1 {
		s[l], s[r] = s[r], s[l]
	}
	return "User " + string(s)
}
