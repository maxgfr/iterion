package marketplace

import "testing"

func TestEffectiveHelpers_LegacyZeroValues(t *testing.T) {
	// A legacy flat entry (no scope/status/source set) must read as
	// public + approved + git so it stays world-visible with no migration.
	var e Entry
	if got := EffectiveStatus(e); got != StatusApproved {
		t.Errorf("EffectiveStatus zero = %q, want approved", got)
	}
	if got := EffectiveScope(e); got != ScopePublic {
		t.Errorf("EffectiveScope zero = %q, want public", got)
	}
	if got := EffectiveSource(e); got != SourceGit {
		t.Errorf("EffectiveSource zero = %q, want git", got)
	}
}

func TestVisible(t *testing.T) {
	approvedPublic := Entry{Slug: "a", Status: StatusApproved, Scope: ScopePublic}
	legacy := Entry{Slug: "legacy"} // empty status/scope → approved/public
	approvedInstance := Entry{Slug: "i", Status: StatusApproved, Scope: ScopeInstance}
	approvedOrgX := Entry{Slug: "ox", Status: StatusApproved, Scope: ScopeOrg, OrgID: "X"}
	pendingPublicOwnedByU := Entry{Slug: "p", Status: StatusPending, Scope: ScopePublic, SubmittedBy: "U"}

	anon := ViewerContext{Enforce: true}
	authedNoOrg := ViewerContext{Enforce: true, Authenticated: true, UserID: "U"}
	authedOrgX := ViewerContext{Enforce: true, Authenticated: true, UserID: "V", OrgIDs: []string{"X"}}
	authedOrgY := ViewerContext{Enforce: true, Authenticated: true, UserID: "W", OrgIDs: []string{"Y"}}
	superAdmin := ViewerContext{Enforce: true, Authenticated: true, UserID: "S", IsSuperAdmin: true}
	local := ViewerContext{} // Enforce false

	cases := []struct {
		name string
		e    Entry
		v    ViewerContext
		want bool
	}{
		{"local sees everything (pending)", pendingPublicOwnedByU, local, true},
		{"anon sees approved public", approvedPublic, anon, true},
		{"anon sees legacy", legacy, anon, true},
		{"anon cannot see instance", approvedInstance, anon, false},
		{"anon cannot see org", approvedOrgX, anon, false},
		{"anon cannot see pending", pendingPublicOwnedByU, anon, false},
		{"authed sees instance", approvedInstance, authedNoOrg, true},
		{"authed without org cannot see org", approvedOrgX, authedNoOrg, false},
		{"org member sees own org", approvedOrgX, authedOrgX, true},
		{"other org cannot see org", approvedOrgX, authedOrgY, false},
		{"owner sees own pending", pendingPublicOwnedByU, authedNoOrg, true},
		{"non-owner cannot see pending", pendingPublicOwnedByU, authedOrgY, false},
		{"super-admin sees any org approved", approvedOrgX, superAdmin, true},
		{"super-admin still cannot see foreign pending via browse", pendingPublicOwnedByU, superAdmin, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Visible(tc.e, tc.v); got != tc.want {
				t.Errorf("Visible(%s, %+v) = %v, want %v", tc.e.Slug, tc.v, got, tc.want)
			}
		})
	}
}
