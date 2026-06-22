package orgsso

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"@Acme.COM":          "acme.com",
		"  acme.com  ":       "acme.com",
		"*.acme.com":         "acme.com",
		"https://acme.com/x": "acme.com",
		"acme.com:8443":      "acme.com",
		"sso.acme.com":       "sso.acme.com",
	}
	for in, want := range cases {
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailDomain(t *testing.T) {
	if got := EmailDomain("Alice@Acme.com"); got != "acme.com" {
		t.Errorf("got %q", got)
	}
	if got := EmailDomain("no-at-sign"); got != "" {
		t.Errorf("expected empty for bad email, got %q", got)
	}
}

func TestMemoryDomainStore_CRUDAndVerify(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryDomainStore()
	tok, _ := NewDomainToken()
	d := VerifiedDomain{ID: "d1", TenantID: "t1", Domain: "Acme.com", Token: tok, CreatedAt: time.Now()}
	if err := st.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Stored normalized; not yet verified.
	got, _ := st.Get(ctx, "d1")
	if got.Domain != "acme.com" || got.Verified() {
		t.Fatalf("got %+v", got)
	}
	if ok, _ := st.IsVerifiedForTenant(ctx, "t1", "acme.com"); ok {
		t.Errorf("should not be verified yet")
	}
	// Duplicate domain for same tenant → ErrDomainExists.
	if err := st.Create(ctx, VerifiedDomain{ID: "d2", TenantID: "t1", Domain: "acme.com", Token: "x"}); !errors.Is(err, ErrDomainExists) {
		t.Errorf("dup: want ErrDomainExists, got %v", err)
	}
	// Mark verified.
	now := time.Now()
	got.VerifiedAt = &now
	if err := st.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	if ok, _ := st.IsVerifiedForTenant(ctx, "t1", "acme.com"); !ok {
		t.Errorf("should be verified after update")
	}
	// Tenant isolation: another tenant's lookup of the same domain is false.
	if ok, _ := st.IsVerifiedForTenant(ctx, "t2", "acme.com"); ok {
		t.Errorf("cross-tenant verified leak")
	}
}

func TestVerifyDomainTXT(t *testing.T) {
	d := VerifiedDomain{Domain: "acme.com", Token: "tok123"}
	ctx := context.Background()
	// Records carry the expected challenge → verified.
	ok, err := VerifyDomainTXT(ctx, func(_ context.Context, name string) ([]string, error) {
		if name != "_iterion-challenge.acme.com" {
			t.Errorf("looked up %q", name)
		}
		return []string{"some-other-record", "iterion-site-verification=tok123"}, nil
	}, d)
	if err != nil || !ok {
		t.Errorf("expected verified, got ok=%v err=%v", ok, err)
	}
	// Wrong token → not verified.
	ok, _ = VerifyDomainTXT(ctx, func(_ context.Context, _ string) ([]string, error) {
		return []string{"iterion-site-verification=WRONG"}, nil
	}, d)
	if ok {
		t.Errorf("wrong token should not verify")
	}
	// Lookup error propagates.
	if _, err := VerifyDomainTXT(ctx, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("nxdomain")
	}, d); err == nil {
		t.Errorf("expected lookup error to propagate")
	}
}
