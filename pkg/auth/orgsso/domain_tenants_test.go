package orgsso

import (
	"context"
	"testing"
	"time"
)

func TestMemoryDomainStore_TenantsForDomain(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryDomainStore()
	now := time.Now()
	verified := now
	// acme.example verified for two tenants; pending for a third (not returned).
	rows := []VerifiedDomain{
		{ID: "1", TenantID: "t-a", Domain: "acme.example", Token: "x", VerifiedAt: &verified, CreatedAt: now},
		{ID: "2", TenantID: "t-b", Domain: "acme.example", Token: "y", VerifiedAt: &verified, CreatedAt: now},
		{ID: "3", TenantID: "t-c", Domain: "acme.example", Token: "z", CreatedAt: now}, // unverified
		{ID: "4", TenantID: "t-a", Domain: "other.example", Token: "w", VerifiedAt: &verified, CreatedAt: now},
	}
	for _, d := range rows {
		if err := st.Create(ctx, d); err != nil {
			t.Fatalf("create %s: %v", d.ID, err)
		}
	}

	got, err := st.TenantsForDomain(ctx, "ACME.example") // case-insensitive
	if err != nil {
		t.Fatalf("TenantsForDomain: %v", err)
	}
	if len(got) != 2 || got[0] != "t-a" || got[1] != "t-b" {
		t.Fatalf("tenants = %v, want [t-a t-b]", got)
	}

	// Unclaimed domain → empty, never an error (non-oracle).
	got, err = st.TenantsForDomain(ctx, "unknown.example")
	if err != nil || len(got) != 0 {
		t.Fatalf("unclaimed domain = %v, %v", got, err)
	}
	if got, _ := st.TenantsForDomain(ctx, ""); len(got) != 0 {
		t.Fatalf("empty domain should yield none, got %v", got)
	}
}
