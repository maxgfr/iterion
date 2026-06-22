package cloudsched

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestCron(t *testing.T) {
	if err := ValidateCron("0 2 * * 1"); err != nil {
		t.Errorf("valid weekly cron rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); err == nil {
		t.Error("invalid cron accepted")
	}
	// 0 2 * * 1 = Mondays 02:00. From a Sunday, next is the coming Monday 02:00.
	from := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) // Sunday
	next, err := NextFire("0 2 * * 1", from)
	if err != nil {
		t.Fatalf("NextFire: %v", err)
	}
	if next.Weekday() != time.Monday || next.Hour() != 2 {
		t.Errorf("next fire: want Monday 02:00, got %s", next)
	}
}

// TestTicker_ExactlyOnce runs many Ticks concurrently against the same due
// schedules and asserts each slot fires exactly once — the multi-replica CAS
// guarantee, no leader.
func TestTicker_ExactlyOnce(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 22, 10, 0, 30, 0, time.UTC)
	past := now.Add(-time.Minute)
	for _, id := range []string{"s1", "s2", "s3"} {
		if err := store.Create(context.Background(), ScheduledBot{
			ID: id, TenantID: "t1", BotID: "sec-audit-source", Cron: "* * * * *", NextFireAt: past,
		}); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	fires := map[string]int{}
	ticker := &Ticker{
		Store: store,
		Now:   func() time.Time { return now },
		Launch: func(_ context.Context, sb ScheduledBot) error {
			mu.Lock()
			fires[sb.ID]++
			mu.Unlock()
			return nil
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = ticker.Tick(context.Background()) }()
	}
	wg.Wait()

	for _, id := range []string{"s1", "s2", "s3"} {
		if fires[id] != 1 {
			t.Errorf("schedule %s fired %d times, want exactly 1", id, fires[id])
		}
	}

	// After firing, none are due again at the same `now` (next_fire_at advanced).
	due, _ := store.ListDue(context.Background(), now, 0)
	if len(due) != 0 {
		t.Errorf("schedules should not be due again after firing, got %d", len(due))
	}
}

func TestTicker_DisabledNotFired(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	_ = store.Create(context.Background(), ScheduledBot{
		ID: "off", TenantID: "t1", BotID: "x", Cron: "* * * * *", NextFireAt: now.Add(-time.Hour), Disabled: true,
	})
	var fired int32
	ticker := &Ticker{Store: store, Now: func() time.Time { return now },
		Launch: func(context.Context, ScheduledBot) error { atomic.AddInt32(&fired, 1); return nil }}
	n, _ := ticker.Tick(context.Background())
	if n != 0 || atomic.LoadInt32(&fired) != 0 {
		t.Errorf("disabled schedule must not fire, n=%d fired=%d", n, fired)
	}
}

func TestMongoStore_CAS(t *testing.T) {
	uri := os.Getenv("ITERION_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("ITERION_TEST_MONGO_URI not set; skipping Mongo cloudsched suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	nonce := make([]byte, 4)
	_, _ = rand.Read(nonce)
	db := client.Database("iterion_cloudsched_" + hex.EncodeToString(nonce))
	t.Cleanup(func() {
		drop, dc := context.WithTimeout(context.Background(), 10*time.Second)
		defer dc()
		_ = db.Drop(drop)
		_ = client.Disconnect(drop)
	})
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema idempotent: %v", err)
	}
	store := NewMongoStore(db)

	now := time.Now().UTC().Truncate(time.Millisecond)
	sb := ScheduledBot{ID: "sb-1", TenantID: "t1", RepoIntegrationID: "ri-1", BotID: "seki",
		Cron: "* * * * *", NextFireAt: now.Add(-time.Minute)}
	if err := store.Create(ctx, sb); err != nil {
		t.Fatalf("Create: %v", err)
	}

	due, err := store.ListDue(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].ID != "sb-1" {
		t.Fatalf("ListDue: %+v err=%v", due, err)
	}
	expected := due[0].NextFireAt
	newNext := now.Add(time.Minute)

	// First CAS wins.
	won, err := store.ClaimTick(ctx, "sb-1", expected, newNext, now)
	if err != nil || !won {
		t.Fatalf("first ClaimTick: won=%v err=%v", won, err)
	}
	// Second CAS on the SAME expected loses (next_fire_at already advanced).
	won2, err := store.ClaimTick(ctx, "sb-1", expected, newNext, now)
	if err != nil || won2 {
		t.Fatalf("second ClaimTick must lose: won=%v err=%v", won2, err)
	}
	got, _ := store.Get(ctx, "sb-1")
	if !got.NextFireAt.Equal(newNext) || got.LastFireAt == nil {
		t.Errorf("post-claim state: %+v", got)
	}

	if err := store.Delete(ctx, "sb-1"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "sb-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: want ErrNotFound, got %v", err)
	}
}
