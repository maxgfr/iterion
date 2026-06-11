package audit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func runStoreSuite(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	seed := []Event{
		{ID: "1", Scope: ScopeTenant, TenantID: "t1", ActorID: "u1", Action: "webhook.created", CreatedAt: base},
		{ID: "2", Scope: ScopeTenant, TenantID: "t1", ActorID: "u2", Action: "secret.created", CreatedAt: base.Add(time.Minute)},
		{ID: "3", Scope: ScopeTenant, TenantID: "t2", ActorID: "u1", Action: "webhook.created", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "4", Scope: ScopePlatform, TenantID: "t1", ActorID: "root", Action: "org.status_changed", CreatedAt: base.Add(3 * time.Minute)},
	}
	for _, e := range seed {
		if err := s.Insert(ctx, e); err != nil {
			t.Fatalf("insert %s: %v", e.ID, err)
		}
	}

	t.Run("tenant scope + isolation", func(t *testing.T) {
		got, err := s.ListByTenant(ctx, "t1", Page{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("t1 events = %d, want 2 (no cross-tenant, no platform leak)", len(got))
		}
		if got[0].ID != "2" || got[1].ID != "1" {
			t.Fatalf("order = %s,%s want newest first 2,1", got[0].ID, got[1].ID)
		}
	})

	t.Run("platform scope", func(t *testing.T) {
		got, err := s.ListPlatform(ctx, Page{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "4" {
			t.Fatalf("platform events = %+v, want only #4", got)
		}
	})

	t.Run("filters", func(t *testing.T) {
		byAction, _ := s.ListByTenant(ctx, "t1", Page{Action: "webhook.created"})
		if len(byAction) != 1 || byAction[0].ID != "1" {
			t.Fatalf("action filter = %+v", byAction)
		}
		byActor, _ := s.ListByTenant(ctx, "t1", Page{ActorID: "u2"})
		if len(byActor) != 1 || byActor[0].ID != "2" {
			t.Fatalf("actor filter = %+v", byActor)
		}
		byTime, _ := s.ListByTenant(ctx, "t1", Page{From: base.Add(30 * time.Second)})
		if len(byTime) != 1 || byTime[0].ID != "2" {
			t.Fatalf("from filter = %+v", byTime)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		page1, _ := s.ListByTenant(ctx, "t1", Page{Limit: 1})
		if len(page1) != 1 || page1[0].ID != "2" {
			t.Fatalf("page1 = %+v", page1)
		}
		page2, _ := s.ListByTenant(ctx, "t1", Page{Offset: 1, Limit: 1})
		if len(page2) != 1 || page2[0].ID != "1" {
			t.Fatalf("page2 = %+v", page2)
		}
		empty, _ := s.ListByTenant(ctx, "t1", Page{Offset: 10})
		if len(empty) != 0 {
			t.Fatalf("past-the-end = %+v", empty)
		}
	})
}

func TestMemoryStore(t *testing.T) { runStoreSuite(t, NewMemoryStore()) }

func TestClampLimit(t *testing.T) {
	for _, c := range []struct{ in, want int }{{0, 50}, {-1, 50}, {10, 10}, {501, 500}} {
		t.Run(fmt.Sprint(c.in), func(t *testing.T) {
			if got := ClampLimit(c.in); got != c.want {
				t.Fatalf("ClampLimit(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
