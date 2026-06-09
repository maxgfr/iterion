package knowledge

import (
	"errors"
	"testing"
)

func TestSpaceRef_IDDeterministicAndDistinct(t *testing.T) {
	a := SpaceRef{Visibility: VisibilityBot, TenantID: "t1", ProjectID: "p1", BotID: "revi", Name: "findings"}
	b := SpaceRef{Visibility: VisibilityBot, TenantID: "t1", ProjectID: "p1", BotID: "revi", Name: "findings"}
	if a.ID() != b.ID() {
		t.Fatalf("equal refs produced different ids: %q vs %q", a.ID(), b.ID())
	}
	// Changing any axis must change the id (no collisions across axes).
	variants := []SpaceRef{
		{Visibility: VisibilityProject, TenantID: "t1", ProjectID: "p1", BotID: "revi", Name: "findings"},
		{Visibility: VisibilityBot, TenantID: "t2", ProjectID: "p1", BotID: "revi", Name: "findings"},
		{Visibility: VisibilityBot, TenantID: "t1", ProjectID: "p2", BotID: "revi", Name: "findings"},
		{Visibility: VisibilityBot, TenantID: "t1", ProjectID: "p1", BotID: "billy", Name: "findings"},
		{Visibility: VisibilityBot, TenantID: "t1", ProjectID: "p1", BotID: "revi", Name: "notes"},
	}
	for _, v := range variants {
		if v.ID() == a.ID() {
			t.Fatalf("ref %+v collided with base id %q", v, a.ID())
		}
	}
}

func TestSpaceRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ref     SpaceRef
		wantErr bool
	}{
		{"bot ok", SpaceRef{Visibility: VisibilityBot, ProjectID: "p1", Name: "findings"}, false},
		{"project ok", SpaceRef{Visibility: VisibilityProject, ProjectID: "p1", Name: "findings"}, false},
		{"user ok", SpaceRef{Visibility: VisibilityUser, UserID: "u1", Name: "notes"}, false},
		{"org ok", SpaceRef{Visibility: VisibilityOrg, Name: "conventions"}, false},
		{"unknown visibility", SpaceRef{Visibility: "weird", Name: "x"}, true},
		{"empty name", SpaceRef{Visibility: VisibilityOrg, Name: ""}, true},
		{"traversal name", SpaceRef{Visibility: VisibilityOrg, Name: "../escape"}, true},
		{"slash name", SpaceRef{Visibility: VisibilityOrg, Name: "a/b"}, true},
		{"bot without project", SpaceRef{Visibility: VisibilityBot, Name: "findings"}, true},
		{"user without user id", SpaceRef{Visibility: VisibilityUser, Name: "notes"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ref.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestQuotaError_IsTargets(t *testing.T) {
	space := &QuotaError{Aggregate: false, Used: 10, Delta: 5, Quota: 12}
	org := &QuotaError{Aggregate: true, Used: 10, Delta: 5, Quota: 12}
	if !errors.Is(space, ErrQuotaExceeded) || errors.Is(space, ErrOrgQuotaExceeded) {
		t.Fatalf("per-space QuotaError matched the wrong sentinel")
	}
	if !errors.Is(org, ErrOrgQuotaExceeded) || errors.Is(org, ErrQuotaExceeded) {
		t.Fatalf("org QuotaError matched the wrong sentinel")
	}
}

func TestDefaultQuotaFor(t *testing.T) {
	if DefaultQuotaFor(VisibilityBot) != DefaultQuotaBot {
		t.Fatalf("bot default mismatch")
	}
	if DefaultQuotaFor(VisibilityGlobal) != 0 {
		t.Fatalf("global should be 0 (read-only through the org path)")
	}
}
