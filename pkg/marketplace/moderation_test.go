package marketplace

import (
	"context"
	"errors"
	"testing"
)

func seedEntry(t *testing.T, s *JSONStore, slug string, scope Scope, status Status, org, submitter string) {
	t.Helper()
	e := Entry{
		Slug:        slug,
		Name:        slug,
		RepoURL:     "https://example.com/" + slug + ".git",
		Scope:       scope,
		Status:      status,
		OrgID:       org,
		SubmittedBy: submitter,
		CreatedAt:   "2026-06-24T00:00:00Z",
		UpdatedAt:   "2026-06-24T00:00:00Z",
	}
	if err := s.Upsert(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

func TestJSONStore_SetStatus_CASGuard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedEntry(t, s, "bot", ScopePublic, StatusPending, "", "U")

	// A guard that doesn't match the current status fails with conflict.
	err := s.SetStatus(ctx, "bot", StatusApproved, StatusRejected, Review{By: "admin", At: "now"})
	if !errors.Is(err, ErrStatusConflict) {
		t.Fatalf("expected ErrStatusConflict, got %v", err)
	}

	// The correct guard (pending) succeeds and stamps review metadata.
	if err := s.SetStatus(ctx, "bot", StatusPending, StatusApproved, Review{By: "admin", At: "ts"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got, _, _ := s.Get(ctx, "bot")
	if got.Status != StatusApproved || got.ReviewedBy != "admin" || got.ReviewedAt != "ts" {
		t.Fatalf("unexpected entry after approve: %+v", got)
	}

	// Rejection records the reason; approval clears it.
	if err := s.SetStatus(ctx, "bot", "", StatusRejected, Review{By: "admin", At: "ts2", Reason: "nope"}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	got, _, _ = s.Get(ctx, "bot")
	if got.Status != StatusRejected || got.RejectReason != "nope" {
		t.Fatalf("reject did not stamp reason: %+v", got)
	}
}

func TestJSONStore_ListForModeration_Scoping(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedEntry(t, s, "pub-pending", ScopePublic, StatusPending, "", "U")
	seedEntry(t, s, "orgX-pending", ScopeOrg, StatusPending, "X", "V")
	seedEntry(t, s, "orgY-pending", ScopeOrg, StatusPending, "Y", "W")
	seedEntry(t, s, "approved", ScopePublic, StatusApproved, "", "U")

	// Org X admin sees only org X's pending.
	got, err := s.ListForModeration(ctx, ModerationQuery{OrgIDs: []string{"X"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slug != "orgX-pending" {
		t.Fatalf("org X moderation = %v", slugs(got))
	}

	// Super-admin (All) sees every pending, never the approved one.
	got, err = s.ListForModeration(ctx, ModerationQuery{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("super-admin moderation = %v, want 3 pending", slugs(got))
	}
	for _, e := range got {
		if EffectiveStatus(e) != StatusPending {
			t.Errorf("non-pending in moderation queue: %s", e.Slug)
		}
	}
}

func TestJSONStore_List_ViewerFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedEntry(t, s, "pub", ScopePublic, StatusApproved, "", "U")
	seedEntry(t, s, "inst", ScopeInstance, StatusApproved, "", "U")
	seedEntry(t, s, "pending", ScopePublic, StatusPending, "", "U")

	// Anonymous enforced viewer: only approved public.
	got, _ := s.List(ctx, Query{Viewer: ViewerContext{Enforce: true}})
	if len(got) != 1 || got[0].Slug != "pub" {
		t.Fatalf("anon list = %v, want [pub]", slugs(got))
	}

	// Local (zero viewer): everything, including pending.
	got, _ = s.List(ctx, Query{})
	if len(got) != 3 {
		t.Fatalf("local list = %v, want all 3", slugs(got))
	}
}

func TestJSONStore_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedEntry(t, s, "gone", ScopePublic, StatusApproved, "", "U")
	if err := s.Delete(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get(ctx, "gone"); ok {
		t.Fatal("entry still present after delete")
	}
	// Missing slug is a no-op, not an error.
	if err := s.Delete(ctx, "never"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func slugs(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Slug
	}
	return out
}
