package secrets

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// seedRecord seals a minimal credentials.json (with a refresh token) for
// ownerKey/kind and stores it in st, expiring at exp.
func seedRecord(t *testing.T, st OAuthStore, sealer Sealer, ownerKey string, kind OAuthKind, exp time.Time) {
	t.Helper()
	blob := []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-old1234567890abcdef","refreshToken":"rf-old","expiresAt":0}}`)
	sealed, err := SealOAuthPayload(sealer, ownerKey, kind, blob)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	e := exp
	if err := st.Upsert(context.Background(), OAuthRecord{
		UserID:               ownerKey,
		Kind:                 kind,
		SealedPayload:        sealed,
		AccessTokenExpiresAt: &e,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func TestOAuthRefreshWorker_RefreshesPersonalAndOrg(t *testing.T) {
	freshRetrySchedule(t)
	sealer, err := NewAESGCMSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	st := NewMemoryOAuthStore()
	// One personal record + one org record, both expiring soon.
	soon := time.Now().Add(5 * time.Minute)
	seedRecord(t, st, sealer, "alice", OAuthKindClaudeCode, soon)
	seedRecord(t, st, sealer, OrgOwnerKey("team1"), OAuthKindClaudeCode, soon)
	// A record far from expiry must NOT be touched.
	seedRecord(t, st, sealer, "bob", OAuthKindClaudeCode, time.Now().Add(48*time.Hour))

	srv := newFakeOAuthServer(`{"access_token":"sk-ant-fresh1234567890abcdef","refresh_token":"rf-fresh","expires_in":28800}`, http.StatusOK)
	defer srv.Close()

	w := &OAuthRefreshWorker{
		Store:             st,
		Sealer:            sealer,
		HTTP:              redirectingClient(srv.URL),
		AnthropicClientID: "client-xyz",
		Lead:              30 * time.Minute,
	}
	n, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Fatalf("refreshed count: got %d want 2 (alice + org)", n)
	}
	// Both refreshed records now carry the fresh access token + a bumped
	// LastRefreshedAt.
	for _, owner := range []string{"alice", OrgOwnerKey("team1")} {
		rec, err := st.Get(context.Background(), owner, OAuthKindClaudeCode)
		if err != nil {
			t.Fatalf("get %s: %v", owner, err)
		}
		if rec.LastRefreshedAt == nil {
			t.Errorf("%s: LastRefreshedAt not set", owner)
		}
		blob, err := OpenOAuthPayload(sealer, owner, OAuthKindClaudeCode, rec.SealedPayload)
		if err != nil {
			t.Fatalf("unseal %s: %v", owner, err)
		}
		v, _ := ParseAnthropicView(blob)
		if v.ClaudeAIOauth.AccessToken != "sk-ant-fresh1234567890abcdef" {
			t.Errorf("%s: token not rotated: %q", owner, v.ClaudeAIOauth.AccessToken)
		}
	}
	// bob (far from expiry) untouched.
	bob, _ := st.Get(context.Background(), "bob", OAuthKindClaudeCode)
	if bob.LastRefreshedAt != nil {
		t.Error("bob should not have been refreshed")
	}
}

func TestOAuthRefreshWorker_SkipsKindWithoutClientID(t *testing.T) {
	sealer, _ := NewAESGCMSealer(make([]byte, 32))
	st := NewMemoryOAuthStore()
	seedRecord(t, st, sealer, "alice", OAuthKindClaudeCode, time.Now().Add(time.Minute))
	// No Anthropic client id configured → nothing refreshed, no error.
	w := &OAuthRefreshWorker{Store: st, Sealer: sealer, HTTP: http.DefaultClient}
	n, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 refreshed, got %d", n)
	}
}
