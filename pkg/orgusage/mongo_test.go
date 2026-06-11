package orgusage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestMongoCounter runs the shared Counter suite against a real Mongo
// (same gating as the pkg/store/mongo conformance harness — the CI
// mongo-conformance job sets ITERION_TEST_MONGO_URI; local runs use
// `task cloud:up:deps`).
func TestMongoCounter(t *testing.T) {
	uri := os.Getenv("ITERION_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("ITERION_TEST_MONGO_URI not set; skipping Mongo orgusage suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	nonce := make([]byte, 4)
	_, _ = rand.Read(nonce)
	db := client.Database("iterion_orgusage_" + hex.EncodeToString(nonce))
	t.Cleanup(func() {
		drop, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(drop)
		_ = client.Disconnect(drop)
	})
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// Idempotency: a second EnsureSchema must not error.
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema (second): %v", err)
	}
	runCounterSuite(t, NewMongoCounter(db))
}
