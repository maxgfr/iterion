package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/SocialGouv/iterion/pkg/store"
)

// withoutTenantFilterKey is the ctx key that lets cluster-level admin
// paths (migration tools, the queue dispatcher's bootstrap probe,
// conformance tests) opt back into a tenant-less query. Without this
// flag, a ctx with no TenantID makes withTenantFilter panic — fail-
// closed is the safer default because every business-logic call site
// already runs under a tenant-stamped ctx.
type withoutTenantFilterKey struct{}

// WithoutTenantFilter marks ctx as exempt from the withTenantFilter
// guard. Use sparingly — every callsite is a potential tenant-isolation
// hole. Audit by grepping the codebase for callers.
func WithoutTenantFilter(ctx context.Context) context.Context {
	return context.WithValue(ctx, withoutTenantFilterKey{}, true)
}

func isWithoutTenantFilter(ctx context.Context) bool {
	v, _ := ctx.Value(withoutTenantFilterKey{}).(bool)
	return v
}

// withTenantFilter augments a Mongo filter with a tenant_id clause
// derived from ctx. Fail-closed: when ctx carries no tenant AND the
// caller has not explicitly opted out via WithoutTenantFilter, this
// panics. Panicking (rather than returning an error) is the strict
// reading of the audit's "fail-closed" sketch — a missed tenant_id
// is always a bug, never a runtime condition we want to recover from,
// and recoverMutator-style wrappers convert the panic into a 500.
func withTenantFilter(ctx context.Context, base bson.M) bson.M {
	tenantID, ok := store.TenantFromContext(ctx)
	if !ok || tenantID == "" {
		if isWithoutTenantFilter(ctx) {
			return base
		}
		panic(fmt.Errorf("store/mongo: tenant-scoped query without tenant in ctx (use WithoutTenantFilter to bypass)"))
	}
	out := make(bson.M, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["tenant_id"] = tenantID
	return out
}

// stampTenant copies the tenant_id from ctx onto a Run before
// persisting it. Idempotent: an already-set tenant_id is preserved.
func stampTenant(ctx context.Context, r *store.Run) {
	if r == nil {
		return
	}
	if r.TenantID != "" {
		return
	}
	if id, ok := store.TenantFromContext(ctx); ok {
		r.TenantID = id
	}
	if r.OwnerID == "" {
		if uid, ok := store.OwnerFromContext(ctx); ok {
			r.OwnerID = uid
		}
	}
}

// stampTenantOnEvent does the same for Event documents.
func stampTenantOnEvent(ctx context.Context, e *store.Event) {
	if e == nil || e.TenantID != "" {
		return
	}
	if id, ok := store.TenantFromContext(ctx); ok {
		e.TenantID = id
	}
}

// stampTenantOnInteraction does the same for Interaction documents.
func stampTenantOnInteraction(ctx context.Context, i *store.Interaction) {
	if i == nil || i.TenantID != "" {
		return
	}
	if id, ok := store.TenantFromContext(ctx); ok {
		i.TenantID = id
	}
}
