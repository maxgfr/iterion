package store

import (
	"context"
	"testing"
)

// wantCtxVal asserts a (value, ok) pair returned by TenantFromContext /
// OwnerFromContext. ok must be true exactly when a non-empty value was
// stamped — an empty want encodes the "no value set" contract.
func wantCtxVal(t *testing.T, label, got string, ok bool, want string) {
	t.Helper()
	if got != want || ok != (want != "") {
		t.Errorf("%s = (%q, %v), want (%q, %v)", label, got, ok, want, want != "")
	}
}

func TestWithTenantRoundTrip(t *testing.T) {
	got, ok := TenantFromContext(WithTenant(context.Background(), "team-1"))
	wantCtxVal(t, "TenantFromContext", got, ok, "team-1")
}

// An empty tenantID must be a no-op: the returned context carries no
// tenant value, so the mongo guard treats the call as privileged rather
// than scoping it to "".
func TestWithTenantEmptyReturnsParent(t *testing.T) {
	got, ok := TenantFromContext(WithTenant(context.Background(), ""))
	wantCtxVal(t, "WithTenant(_, \"\")", got, ok, "")
}

func TestTenantFromContextEmptyAndNil(t *testing.T) {
	got, ok := TenantFromContext(context.Background())
	wantCtxVal(t, "bare ctx", got, ok, "")
	got, ok = TenantFromContext(nil)
	wantCtxVal(t, "nil ctx", got, ok, "")
}

func TestWithOwnerRoundTripAndEmpty(t *testing.T) {
	got, ok := OwnerFromContext(WithOwner(context.Background(), "user-1"))
	wantCtxVal(t, "OwnerFromContext", got, ok, "user-1")

	got, ok = OwnerFromContext(WithOwner(context.Background(), ""))
	wantCtxVal(t, "WithOwner(_, \"\")", got, ok, "")

	got, ok = OwnerFromContext(nil)
	wantCtxVal(t, "nil ctx", got, ok, "")
}

// WithIdentity must stamp both slots independently — a regression that
// stored both under one key (or swapped them) would surface here.
func TestWithIdentityStampsBoth(t *testing.T) {
	ctx := WithIdentity(context.Background(), "team-9", "user-9")

	tid, tok := TenantFromContext(ctx)
	wantCtxVal(t, "TenantFromContext", tid, tok, "team-9")
	oid, ook := OwnerFromContext(ctx)
	wantCtxVal(t, "OwnerFromContext", oid, ook, "user-9")

	// The two values must not bleed into each other's slot.
	if tid == oid {
		t.Errorf("tenant and owner resolved to the same value %q — keys collided", tid)
	}
}

func TestWithoutTenantFilter(t *testing.T) {
	if IsWithoutTenantFilter(context.Background()) {
		t.Error("plain ctx: IsWithoutTenantFilter = true, want false (fail-closed default)")
	}
	if IsWithoutTenantFilter(nil) {
		t.Error("nil ctx: IsWithoutTenantFilter = true, want false")
	}
	if !IsWithoutTenantFilter(WithoutTenantFilter(context.Background())) {
		t.Error("after WithoutTenantFilter: IsWithoutTenantFilter = false, want true")
	}
}
