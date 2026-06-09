package memory

import (
	"context"
	"testing"

	"github.com/SocialGouv/iterion/pkg/knowledge"
)

func TestResolveSpaceRef(t *testing.T) {
	in := SpaceRefInputs{TenantID: "acme", UserID: "alice", ProjectID: "pkey", BotID: "revi"}

	bot := ResolveSpaceRef(knowledge.VisibilityBot, "x", "", "", in)
	if bot.ProjectID != "pkey" || bot.BotID != "revi" {
		t.Fatalf("bot: %+v", bot)
	}
	user := ResolveSpaceRef(knowledge.VisibilityUser, "x", "", "", in)
	if user.UserID != "alice" {
		t.Fatalf("user: %+v", user)
	}
	if local := ResolveSpaceRef(knowledge.VisibilityUser, "x", "", "", SpaceRefInputs{}); local.UserID != "local" {
		t.Fatalf("user local fallback: %+v", local)
	}
	if userOverride := ResolveSpaceRef(knowledge.VisibilityUser, "x", "", "bob", in); userOverride.UserID != "bob" {
		t.Fatalf("user override: %+v", userOverride)
	}
	org := ResolveSpaceRef(knowledge.VisibilityOrg, "x", "", "", in)
	if org.TenantID != "acme" {
		t.Fatalf("org: %+v", org)
	}
	if glob := ResolveSpaceRef(knowledge.VisibilityGlobal, "x", "", "", in); glob.TenantID != "" {
		t.Fatalf("global must not be tenant-scoped: %+v", glob)
	}
}

func TestFSStore_SharedTreeVisibilities(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s := DefaultFSStore()
	ctx := context.Background()

	refs := []knowledge.SpaceRef{
		{Visibility: knowledge.VisibilityUser, TenantID: "acme", UserID: "alice", Name: "notes"},
		{Visibility: knowledge.VisibilityOrg, TenantID: "acme", Name: "conventions"},
		{Visibility: knowledge.VisibilityCrossProject, TenantID: "acme", Name: "libs"},
		{Visibility: knowledge.VisibilityGlobal, Name: "policy"},
		{Visibility: knowledge.VisibilityBot, ProjectID: "pkey", Name: "findings"},
	}
	roots := map[string]bool{}
	for _, ref := range refs {
		if _, err := s.WriteDocument(ctx, ref, knowledge.DocumentInput{Path: "a.md", Content: []byte("x")}); err != nil {
			t.Fatalf("write %s: %v", ref.Visibility, err)
		}
		doc, err := s.ReadDocument(ctx, ref, "a.md")
		if err != nil || string(doc.Content) != "x" {
			t.Fatalf("read %s: %v", ref.Visibility, err)
		}
		root, err := s.Root(ref)
		if err != nil {
			t.Fatalf("root %s: %v", ref.Visibility, err)
		}
		if roots[root] {
			t.Fatalf("visibility %s collided on path %s", ref.Visibility, root)
		}
		roots[root] = true
	}

	// The org space is shared regardless of which user/project writes it.
	orgRef := knowledge.SpaceRef{Visibility: knowledge.VisibilityOrg, TenantID: "acme", Name: "conventions"}
	if doc, _ := s.ReadDocument(ctx, orgRef, "a.md"); string(doc.Content) != "x" {
		t.Fatal("org space not shared across resolutions")
	}

	// empty tenant maps to "local" (single-tenant mode).
	localOrg := knowledge.SpaceRef{Visibility: knowledge.VisibilityOrg, Name: "conv"}
	if _, err := s.WriteDocument(ctx, localOrg, knowledge.DocumentInput{Path: "b.md", Content: []byte("y")}); err != nil {
		t.Fatalf("local org write: %v", err)
	}
	if root, _ := s.Root(localOrg); !contains(root, "tenants/local/org") {
		t.Fatalf("empty tenant should map to local: %s", root)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
