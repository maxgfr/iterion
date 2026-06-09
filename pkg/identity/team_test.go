package identity

import (
	"context"
	"testing"
	"time"
)

func TestTeamStatusHelpers(t *testing.T) {
	cases := []struct {
		status    TeamStatus
		effective TeamStatus
		canLaunch bool
		suspended bool
	}{
		{"", TeamStatusActive, true, false},
		{TeamStatusActive, TeamStatusActive, true, false},
		{TeamStatusReadOnly, TeamStatusReadOnly, false, false},
		{TeamStatusSuspended, TeamStatusSuspended, false, true},
	}
	for _, c := range cases {
		tm := Team{Status: c.status}
		if tm.EffectiveStatus() != c.effective {
			t.Errorf("EffectiveStatus(%q)=%q want %q", c.status, tm.EffectiveStatus(), c.effective)
		}
		if tm.CanLaunch() != c.canLaunch {
			t.Errorf("CanLaunch(%q)=%v want %v", c.status, tm.CanLaunch(), c.canLaunch)
		}
		if tm.Suspended() != c.suspended {
			t.Errorf("Suspended(%q)=%v want %v", c.status, tm.Suspended(), c.suspended)
		}
	}
}

func TestValidTeamStatus(t *testing.T) {
	for _, s := range []TeamStatus{TeamStatusActive, TeamStatusSuspended, TeamStatusReadOnly} {
		if !ValidTeamStatus(s) {
			t.Errorf("want valid: %q", s)
		}
	}
	for _, s := range []TeamStatus{"", "weird", "Active", "SUSPENDED"} {
		if ValidTeamStatus(s) {
			t.Errorf("want invalid: %q", s)
		}
	}
}

func TestMemoryStore_ListTeams(t *testing.T) {
	st := NewMemoryStore()
	ctx := context.Background()
	base := time.Now()
	for i, slug := range []string{"a", "b", "c"} {
		if _, err := st.CreateTeam(ctx, Team{ID: slug, Name: slug, Slug: slug, CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatalf("CreateTeam: %v", err)
		}
	}
	all, err := st.ListTeams(ctx, Page{})
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d teams want 3", len(all))
	}
	if all[0].ID != "a" || all[2].ID != "c" {
		t.Fatalf("oldest-first order broken: %s,%s,%s", all[0].ID, all[1].ID, all[2].ID)
	}
	page, _ := st.ListTeams(ctx, Page{Offset: 1, Limit: 1})
	if len(page) != 1 || page[0].ID != "b" {
		t.Fatalf("pagination: %+v", page)
	}
}
