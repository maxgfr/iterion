package secrets

import (
	"context"
	"testing"
	"time"
)

func TestMemoryOAuthPendingStore_PutTakeRoundTrip(t *testing.T) {
	sealer, err := NewAESGCMSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	st := NewMemoryOAuthPendingStore()
	ctx := context.Background()

	sealed, err := SealOAuthVerifier(sealer, "alice", OAuthKindClaudeCode, "verifier-123")
	if err != nil {
		t.Fatalf("SealOAuthVerifier: %v", err)
	}
	if err := st.Put(ctx, OAuthPending{
		OwnerKey:       "alice",
		Kind:           OAuthKindClaudeCode,
		SealedVerifier: sealed,
		State:          "state-1",
		ExpiresAt:      time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := st.Take(ctx, "alice", OAuthKindClaudeCode)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if got.State != "state-1" {
		t.Errorf("state: got %q", got.State)
	}
	v, err := OpenOAuthVerifier(sealer, "alice", OAuthKindClaudeCode, got.SealedVerifier)
	if err != nil {
		t.Fatalf("OpenOAuthVerifier: %v", err)
	}
	if v != "verifier-123" {
		t.Errorf("verifier: got %q", v)
	}

	// One-shot: a second Take must miss.
	if _, err := st.Take(ctx, "alice", OAuthKindClaudeCode); err != ErrOAuthPendingNotFound {
		t.Errorf("second take: got %v want ErrOAuthPendingNotFound", err)
	}
}

func TestMemoryOAuthPendingStore_ExpiredRejected(t *testing.T) {
	st := NewMemoryOAuthPendingStore()
	ctx := context.Background()
	_ = st.Put(ctx, OAuthPending{
		OwnerKey:  "bob",
		Kind:      OAuthKindClaudeCode,
		State:     "old",
		ExpiresAt: time.Now().Add(-time.Second),
	})
	if _, err := st.Take(ctx, "bob", OAuthKindClaudeCode); err != ErrOAuthPendingNotFound {
		t.Errorf("expired take: got %v want ErrOAuthPendingNotFound", err)
	}
}

func TestMemoryOAuthPendingStore_IsolatedByOwnerKind(t *testing.T) {
	st := NewMemoryOAuthPendingStore()
	ctx := context.Background()
	_ = st.Put(ctx, OAuthPending{OwnerKey: "alice", Kind: OAuthKindClaudeCode, State: "a", ExpiresAt: time.Now().Add(time.Minute)})
	_ = st.Put(ctx, OAuthPending{OwnerKey: OrgOwnerKey("team1"), Kind: OAuthKindClaudeCode, State: "org", ExpiresAt: time.Now().Add(time.Minute)})

	// Taking alice must not consume the org pending.
	if _, err := st.Take(ctx, "alice", OAuthKindClaudeCode); err != nil {
		t.Fatalf("take alice: %v", err)
	}
	got, err := st.Take(ctx, OrgOwnerKey("team1"), OAuthKindClaudeCode)
	if err != nil {
		t.Fatalf("take org: %v", err)
	}
	if got.State != "org" {
		t.Errorf("org state: got %q", got.State)
	}
}
