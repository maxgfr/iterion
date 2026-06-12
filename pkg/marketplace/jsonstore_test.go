package marketplace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *JSONStore {
	t.Helper()
	s, err := NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestJSONStore_UpsertAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	e := Entry{
		Slug:        "feature-dev",
		Name:        "feature_dev",
		DisplayName: "Featurly",
		Description: "ships features end-to-end",
		Author:      "jo",
		Tags:        []string{"impl", "review"},
		RepoURL:     "https://example.com/repo.git",
		CreatedAt:   "2026-06-12T00:00:00Z",
		UpdatedAt:   "2026-06-12T00:00:00Z",
	}
	if err := s.Upsert(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(ctx, "feature-dev")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("entry not found")
	}
	if got.Name != "feature_dev" || got.DisplayName != "Featurly" {
		t.Errorf("entry mismatch: %+v", got)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags: %v", got.Tags)
	}
}

func TestJSONStore_GetMissing(t *testing.T) {
	s := newTestStore(t)
	got, ok, err := s.Get(context.Background(), "nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != nil {
		t.Errorf("expected miss, got %+v", got)
	}
}

func TestJSONStore_UpsertRequiresSlug(t *testing.T) {
	s := newTestStore(t)
	if err := s.Upsert(context.Background(), Entry{Name: "x"}); err == nil {
		t.Fatal("expected error on empty slug")
	}
}

func TestJSONStore_UpsertPreservesInstallsOnRefresh(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Upsert(ctx, Entry{Slug: "a", Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementInstalls(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementInstalls(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	// Re-submit with installs=0 must keep the existing count.
	if err := s.Upsert(ctx, Entry{Slug: "a", Name: "a", Description: "refreshed"}); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := s.Get(ctx, "a")
	if !ok {
		t.Fatal("missing")
	}
	if got.Installs != 2 {
		t.Errorf("installs = %d, want 2", got.Installs)
	}
	if got.Description != "refreshed" {
		t.Errorf("description not updated: %q", got.Description)
	}
}

func TestJSONStore_IncrementInstallsMissing(t *testing.T) {
	s := newTestStore(t)
	if err := s.IncrementInstalls(context.Background(), "nope"); err == nil {
		t.Fatal("expected error on missing slug")
	}
}

func seedListEntries(t *testing.T, s *JSONStore) {
	t.Helper()
	ctx := context.Background()
	entries := []Entry{
		{Slug: "featurly", Name: "feature_dev", DisplayName: "Featurly", Description: "ships features end-to-end", Tags: []string{"impl", "review"}, Author: "jo"},
		{Slug: "billy", Name: "branch_improve_loop", DisplayName: "Billy", Description: "improves a branch", Tags: []string{"review", "loop"}, Author: "jo"},
		{Slug: "revi", Name: "review_pr", DisplayName: "Revi", Description: "reviews PRs", Tags: []string{"review"}, Author: "claude"},
	}
	for _, e := range entries {
		if err := s.Upsert(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSONStore_List(t *testing.T) {
	s := newTestStore(t)
	seedListEntries(t, s)
	ctx := context.Background()

	cases := []struct {
		name     string
		q        Query
		wantSlug []string
	}{
		{
			name:     "no filter returns all",
			q:        Query{},
			wantSlug: []string{"billy", "featurly", "revi"}, // alpha by slug at installs=0
		},
		{
			name:     "text search by description",
			q:        Query{Text: "improves"},
			wantSlug: []string{"billy"},
		},
		{
			name:     "text search by display name (case-insensitive)",
			q:        Query{Text: "feature"},
			wantSlug: []string{"featurly"},
		},
		{
			name:     "text search by author",
			q:        Query{Text: "claude"},
			wantSlug: []string{"revi"},
		},
		{
			name:     "text search via tag substring",
			q:        Query{Text: "loop"},
			wantSlug: []string{"billy"},
		},
		{
			name:     "exact tag filter",
			q:        Query{Tag: "review"},
			wantSlug: []string{"billy", "featurly", "revi"},
		},
		{
			name:     "exact tag filter case-insensitive",
			q:        Query{Tag: "Review"},
			wantSlug: []string{"billy", "featurly", "revi"},
		},
		{
			name:     "tag + text combined",
			q:        Query{Tag: "review", Text: "ships"},
			wantSlug: []string{"featurly"},
		},
		{
			name:     "unknown tag returns nothing",
			q:        Query{Tag: "absent"},
			wantSlug: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.List(ctx, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.wantSlug) {
				t.Fatalf("got %d entries, want %d (%v)", len(got), len(tc.wantSlug), got)
			}
			for i, e := range got {
				if e.Slug != tc.wantSlug[i] {
					t.Errorf("[%d] slug = %q, want %q", i, e.Slug, tc.wantSlug[i])
				}
			}
		})
	}
}

func TestJSONStore_ListSortsByInstalls(t *testing.T) {
	s := newTestStore(t)
	seedListEntries(t, s)
	ctx := context.Background()
	// Bump revi 3x, billy 1x.
	for i := 0; i < 3; i++ {
		if err := s.IncrementInstalls(ctx, "revi"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.IncrementInstalls(ctx, "billy"); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"revi", "billy", "featurly"}
	for i, e := range got {
		if e.Slug != want[i] {
			t.Errorf("[%d] slug = %q, want %q (installs=%d)", i, e.Slug, want[i], e.Installs)
		}
	}
}

func TestJSONStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Upsert(ctx, Entry{Slug: "x", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementInstalls(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	// Re-open and check the entry survived.
	s2, err := NewJSONStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, _ := s2.Get(ctx, "x")
	if !ok {
		t.Fatal("entry lost across reload")
	}
	if got.Installs != 1 {
		t.Errorf("installs = %d, want 1", got.Installs)
	}
}

func TestJSONStore_MalformedFileIsHardError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, jsonStoreFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewJSONStore(dir); err == nil {
		t.Fatal("expected NewJSONStore to fail on malformed file")
	}
}

func TestJSONStore_DefensiveCopies(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	e := Entry{Slug: "x", Name: "x", Tags: []string{"a", "b"}, Presets: []EntryPreset{{Name: "p1", Skills: []string{"s1"}}}}
	if err := s.Upsert(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get(ctx, "x")
	got.Tags[0] = "TAMPERED"
	got.Presets[0].Skills[0] = "TAMPERED"
	got2, _, _ := s.Get(ctx, "x")
	if got2.Tags[0] != "a" {
		t.Errorf("mutation leaked into store: tags = %v", got2.Tags)
	}
	if got2.Presets[0].Skills[0] != "s1" {
		t.Errorf("mutation leaked into store: skills = %v", got2.Presets[0].Skills)
	}
}
