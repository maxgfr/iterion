package main

import (
	"strings"
	"testing"
)

func TestAnonymizer_StablePseudonyms(t *testing.T) {
	a := NewAnonymizer()
	// Same user → same label across calls.
	if got := a.Label("u1"); got != "User A" {
		t.Fatalf("first label = %q, want User A", got)
	}
	if got := a.Label("u2"); got != "User B" {
		t.Fatalf("second label = %q, want User B", got)
	}
	if got := a.Label("u1"); got != "User A" {
		t.Fatalf("u1 label not stable: %q", got)
	}
}

func TestAnonymizer_TranscriptExcludesNonConsenting(t *testing.T) {
	a := NewAnonymizer()
	msgs := []Message{
		{UserID: "u1", Text: "ship it", CreateAtMillis: 1},
		{UserID: "u2", Text: "ship what exactly?", CreateAtMillis: 2},
		{UserID: "u3", Text: "secret aside", CreateAtMillis: 3},
	}
	consent := map[string]bool{"u1": true, "u2": true} // u3 NOT consenting

	got := a.Transcript(msgs, consent)

	if strings.Contains(got, "secret aside") {
		t.Errorf("non-consenting user's content leaked into transcript:\n%s", got)
	}
	if strings.Contains(got, "User C") {
		t.Errorf("non-consenting user got a pseudonym (should be invisible):\n%s", got)
	}
	if !strings.Contains(got, "User A: ship it") {
		t.Errorf("consenting u1 missing:\n%s", got)
	}
	if !strings.Contains(got, "User B: ship what exactly?") {
		t.Errorf("consenting u2 missing:\n%s", got)
	}
}

func TestAnonymizer_TranscriptChronological(t *testing.T) {
	a := NewAnonymizer()
	msgs := []Message{
		{UserID: "u1", Text: "third", CreateAtMillis: 30},
		{UserID: "u1", Text: "first", CreateAtMillis: 10},
		{UserID: "u1", Text: "second", CreateAtMillis: 20},
	}
	got := a.Transcript(msgs, map[string]bool{"u1": true})
	want := "User A: first\nUser A: second\nUser A: third"
	if got != want {
		t.Errorf("transcript order wrong:\ngot  %q\nwant %q", got, want)
	}
}

func TestAnonymizer_NoConsentersYieldsEmpty(t *testing.T) {
	a := NewAnonymizer()
	msgs := []Message{{UserID: "u1", Text: "hi", CreateAtMillis: 1}}
	if got := a.Transcript(msgs, map[string]bool{}); got != "" {
		t.Errorf("expected empty transcript, got %q", got)
	}
}

func TestPseudonymWrapsPastZ(t *testing.T) {
	if got := pseudonym(0); got != "User A" {
		t.Errorf("pseudonym(0) = %q", got)
	}
	if got := pseudonym(25); got != "User Z" {
		t.Errorf("pseudonym(25) = %q", got)
	}
	if got := pseudonym(26); got != "User AA" {
		t.Errorf("pseudonym(26) = %q", got)
	}
}
