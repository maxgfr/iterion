package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SocialGouv/iterion/pkg/marketplace"
)

func writeSeedBundle(t *testing.T, dir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.bot"), []byte("workflow w:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	man := "name: " + name + "\nversion: " + version + "\ndescription: seed bot\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSeedMarketplace_Idempotent_NoClobber(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	bots := filepath.Join(ws, "bots")
	writeSeedBundle(t, filepath.Join(bots, "alpha"), "alpha", "1.0.0")
	writeSeedBundle(t, filepath.Join(bots, "beta"), "beta", "1.0.0")

	store, err := marketplace.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	opts := SeedOptions{Paths: []string{"bots"}, Workdir: ws}

	// First seed indexes both bundles as builtin/approved/public.
	n, err := SeedMarketplace(ctx, store, opts)
	if err != nil || n != 2 {
		t.Fatalf("first seed: n=%d err=%v", n, err)
	}
	a, ok, _ := store.Get(ctx, "alpha")
	if !ok || a.Source != marketplace.SourceBuiltin || a.Status != marketplace.StatusApproved || a.Scope != marketplace.ScopePublic {
		t.Fatalf("alpha not seeded as builtin/approved/public: %+v", a)
	}

	// Reseed with no changes is a no-op.
	if n, _ := SeedMarketplace(ctx, store, opts); n != 0 {
		t.Fatalf("reseed wrote %d, want 0", n)
	}

	// A user entry (git source) with the same slug must never be clobbered.
	if err := store.Upsert(ctx, marketplace.Entry{
		Slug: "beta", Name: "beta", RepoURL: "https://example.com/beta.git",
		Source: marketplace.SourceGit, Version: "9.9.9", Installs: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := SeedMarketplace(ctx, store, opts); n != 0 {
		t.Fatalf("seed clobbered a user entry (wrote %d)", n)
	}
	b, _, _ := store.Get(ctx, "beta")
	if b.Source != marketplace.SourceGit || b.Version != "9.9.9" || b.Installs != 5 {
		t.Fatalf("user beta entry was overwritten: %+v", b)
	}

	// A version drift in a builtin bundle triggers exactly one update,
	// preserving the install counter.
	if err := store.IncrementInstalls(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	writeSeedBundle(t, filepath.Join(bots, "alpha"), "alpha", "2.0.0")
	if n, _ := SeedMarketplace(ctx, store, opts); n != 1 {
		t.Fatalf("version-drift reseed wrote %d, want 1", n)
	}
	a, _, _ = store.Get(ctx, "alpha")
	if a.Version != "2.0.0" {
		t.Errorf("alpha version not updated: %q", a.Version)
	}
	if a.Installs != 1 {
		t.Errorf("install counter not preserved across reseed: %d", a.Installs)
	}
}
