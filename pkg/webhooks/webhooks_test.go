package webhooks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMintAndVerifyToken(t *testing.T) {
	pt, hash, last4, fp, err := MintToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pt, TokenPrefix) {
		t.Fatalf("missing prefix: %q", pt)
	}
	if hash == "" || last4 == "" || fp == "" {
		t.Fatalf("empty derived fields: hash=%q last4=%q fp=%q", hash, last4, fp)
	}
	if !VerifyToken(pt, hash) {
		t.Fatal("verify of the minted token should pass")
	}
	if VerifyToken(pt+"x", hash) {
		t.Fatal("tampered token must fail")
	}
	if VerifyToken("", hash) || VerifyToken(pt, "") {
		t.Fatal("empty inputs must fail")
	}
}

func TestConfig_AllowsBotAndSelect(t *testing.T) {
	scoped := &Config{BotIDs: []string{"review-pr"}}
	if !scoped.AllowsBot("review-pr") || scoped.AllowsBot("other") {
		t.Fatal("scoped allow broken")
	}
	if scoped.SelectBot() != "review-pr" {
		t.Fatal("sole-bot select")
	}
	wild := &Config{BotIDs: []string{"*"}, WildcardBots: true}
	if !wild.AllowsBot("anything") {
		t.Fatal("wildcard allow")
	}
	if wild.SelectBot() != "" {
		t.Fatal("wildcard select must be ambiguous")
	}
	def := &Config{BotIDs: []string{"a", "b"}, DefaultBotID: "b"}
	if def.SelectBot() != "b" {
		t.Fatal("default select")
	}
	if (&Config{BotIDs: []string{"a", "b"}}).SelectBot() != "" {
		t.Fatal("multi without default must be ambiguous")
	}
}

func TestMemoryConfigStore(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryConfigStore()
	c := Config{ID: "w1", TenantID: "t1", Name: "gl", Provider: ProviderGitLab, CreatedAt: time.Now()}
	if err := st.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, c); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate create: %v", err)
	}
	got, err := st.Get(ctx, "w1")
	if err != nil || got.Name != "gl" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if list, _ := st.ListByTenant(ctx, "t1"); len(list) != 1 {
		t.Fatal("list by tenant")
	}
	if other, _ := st.ListByTenant(ctx, "nope"); len(other) != 0 {
		t.Fatal("tenant isolation")
	}
	if err := st.MarkUsed(ctx, "w1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Get(ctx, "w1"); got.LastUsedAt == nil {
		t.Fatal("MarkUsed did not stamp")
	}
	if err := st.Delete(ctx, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestMemoryDeliveryStore_Idempotency(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryDeliveryStore()
	d := Delivery{ID: "d1", TenantID: "t1", WebhookID: "w1", IdempotencyKey: "k1", Status: StatusAccepted, ReceivedAt: time.Now()}
	if err := st.Insert(ctx, d); err != nil {
		t.Fatal(err)
	}
	dup := d
	dup.ID = "d2"
	if err := st.Insert(ctx, dup); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("idempotency dup: %v", err)
	}
	got, err := st.GetByIdempotencyKey(ctx, "k1")
	if err != nil || got.ID != "d1" {
		t.Fatalf("idem lookup: %+v %v", got, err)
	}
	got.Status = StatusLaunched
	got.RunID = "r1"
	if err := st.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.GetByIdempotencyKey(ctx, "k1"); again.Status != StatusLaunched || again.RunID != "r1" {
		t.Fatal("update not reflected")
	}
	if list, _ := st.ListByWebhook(ctx, "t1", "w1", 10); len(list) != 1 {
		t.Fatal("list by webhook")
	}
}

func TestMemoryCounter_OrgAndWebhookCaps(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCounter()
	now := time.Now()

	orgLim := Limits{PerOrgMonthly: 3}
	for i := 0; i < 3; i++ {
		if ok, _ := c.Allow(ctx, "t1", "w1", now, orgLim); !ok {
			t.Fatalf("org call %d should be allowed", i)
		}
	}
	if ok, _ := c.Allow(ctx, "t1", "w1", now, orgLim); ok {
		t.Fatal("4th org call should be denied")
	}
	if n, _ := c.OrgCount(ctx, "t1", now); n != 3 {
		t.Fatalf("denied call must not consume quota, count=%d", n)
	}

	whLim := Limits{PerOrgMonthly: 100, PerWebhookMonthly: 1}
	if ok, _ := c.Allow(ctx, "t2", "wA", now, whLim); !ok {
		t.Fatal("first per-webhook call ok")
	}
	if ok, _ := c.Allow(ctx, "t2", "wA", now, whLim); ok {
		t.Fatal("second per-webhook call denied")
	}
	if ok, _ := c.Allow(ctx, "t2", "wB", now, whLim); !ok {
		t.Fatal("a different webhook under the same org should still be allowed")
	}

	// month rollover resets counters.
	if ok, _ := c.Allow(ctx, "t1", "w1", now.AddDate(0, 1, 0), orgLim); !ok {
		t.Fatal("next month should reset the org counter")
	}
}
