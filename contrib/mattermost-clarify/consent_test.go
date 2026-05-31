package main

import "testing"

func TestConsentStore_ScopedToThread(t *testing.T) {
	c := NewConsentStore()
	c.Set("threadA", "u1", true)

	if !c.HasConsented("threadA", "u1") {
		t.Error("u1 should be consented in threadA")
	}
	// Consent in threadA says nothing about threadB.
	if c.HasConsented("threadB", "u1") {
		t.Error("consent must be thread-scoped, not leak to threadB")
	}
}

func TestConsentStore_DefaultIsExclusion(t *testing.T) {
	c := NewConsentStore()
	if c.HasConsented("t", "never-asked") {
		t.Error("unknown user must default to NOT consented")
	}
	c.Set("t", "u1", false) // declined
	if c.HasConsented("t", "u1") {
		t.Error("declined user must not be consented")
	}
}

func TestConsentStore_ConsentedSet(t *testing.T) {
	c := NewConsentStore()
	c.Set("t", "u1", true)
	c.Set("t", "u2", false)
	c.Set("t", "u3", true)
	set := c.ConsentedSet("t")
	if !set["u1"] || !set["u3"] {
		t.Errorf("u1/u3 should be in consented set: %v", set)
	}
	if set["u2"] {
		t.Errorf("u2 declined, must not be in set: %v", set)
	}
}

func TestConsentStore_PromptOncePerUser(t *testing.T) {
	c := NewConsentStore()
	if !c.NeedsPrompt("t", "u1") {
		t.Error("first sight should need prompt")
	}
	c.MarkPrompted("t", "u1")
	if c.NeedsPrompt("t", "u1") {
		t.Error("already-prompted user should not be re-prompted")
	}
	// Once they answer, still no prompt.
	c.Set("t", "u1", true)
	if c.NeedsPrompt("t", "u1") {
		t.Error("answered user should not be prompted")
	}
}

func TestConsentStore_Activation(t *testing.T) {
	c := NewConsentStore()
	if c.IsActive("t") {
		t.Error("thread should start inactive")
	}
	c.Activate("t")
	if !c.IsActive("t") {
		t.Error("thread should be active after Activate")
	}
}
