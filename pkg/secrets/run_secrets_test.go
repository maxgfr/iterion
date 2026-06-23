package secrets

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/store"
)

func TestMemoryRunSecretsStore_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryRunSecretsStore()
	rec := RunSecretsRecord{
		ID:           "ref-1",
		TenantID:     "t1",
		RunID:        "run-1",
		SealedBundle: []byte("sealed"),
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := st.Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Same tenant → returns the record.
	got, err := st.Get(store.WithTenant(ctx, "t1"), "ref-1")
	if err != nil || got.RunID != "run-1" {
		t.Fatalf("Get under owning tenant: got %+v, err %v", got, err)
	}

	// Different tenant → must look like a missing record (no cross-tenant read).
	if _, err := st.Get(store.WithTenant(ctx, "t2"), "ref-1"); !errors.Is(err, ErrRunSecretsNotFound) {
		t.Fatalf("Get under foreign tenant = %v, want ErrRunSecretsNotFound", err)
	}

	// No tenant in ctx → privileged runner-pickup path returns the record.
	if _, err := st.Get(ctx, "ref-1"); err != nil {
		t.Fatalf("Get under bare ctx (privileged): %v", err)
	}

	// Unknown id → not found.
	if _, err := st.Get(store.WithTenant(ctx, "t1"), "nope"); !errors.Is(err, ErrRunSecretsNotFound) {
		t.Fatalf("Get unknown id = %v, want ErrRunSecretsNotFound", err)
	}
}

func TestMemoryRunSecretsStore_DeleteTenantScoped(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryRunSecretsStore()
	rec := RunSecretsRecord{ID: "ref-1", TenantID: "t1", RunID: "run-1"}
	if err := st.Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Foreign-tenant delete is a silent no-op: returns nil AND leaves the
	// record intact (must not reveal existence, must not delete).
	if err := st.Delete(store.WithTenant(ctx, "t2"), "ref-1"); err != nil {
		t.Fatalf("Delete under foreign tenant returned err %v, want nil no-op", err)
	}
	if _, err := st.Get(store.WithTenant(ctx, "t1"), "ref-1"); err != nil {
		t.Fatalf("record was deleted by a foreign-tenant Delete: %v", err)
	}

	// Owning-tenant delete actually removes it.
	if err := st.Delete(store.WithTenant(ctx, "t1"), "ref-1"); err != nil {
		t.Fatalf("Delete under owning tenant: %v", err)
	}
	if _, err := st.Get(store.WithTenant(ctx, "t1"), "ref-1"); !errors.Is(err, ErrRunSecretsNotFound) {
		t.Fatalf("record still present after owning-tenant Delete: %v", err)
	}
}
