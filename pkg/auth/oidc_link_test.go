package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/identity"
)

// seedUser creates an active user via the identity store directly (bypassing
// signup) so link tests don't depend on signup mode.
func seedUser(t *testing.T, svc *Service, email string) identity.User {
	t.Helper()
	u, err := svc.store.CreateUser(context.Background(), identity.User{
		ID: "u-" + email, Email: identity.NormalizeEmail(email), Status: identity.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func TestLinkExternalToUser_AttachAndIdempotent(t *testing.T) {
	svc := newTestService(t, SignupInviteOnly)
	u := seedUser(t, svc, "alice@acme.example")
	ext := oidc.ExternalUser{Provider: "oidc-org-prov1", Subject: "kc-1", Email: "alice@acme.example"}

	if err := svc.LinkExternalToUser(context.Background(), ext, u.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	// The link now resolves to this user.
	link, err := svc.store.GetOIDCLink(context.Background(), ext.Provider, ext.Subject)
	if err != nil || link.UserID != u.ID {
		t.Fatalf("link not stored for user: %v (%+v)", err, link)
	}
	// Second link is a no-op, not an error.
	if err := svc.LinkExternalToUser(context.Background(), ext, u.ID); err != nil {
		t.Fatalf("re-link should be idempotent: %v", err)
	}
}

func TestLinkExternalToUser_RefusesIdentityOwnedByAnother(t *testing.T) {
	svc := newTestService(t, SignupInviteOnly)
	owner := seedUser(t, svc, "owner@acme.example")
	other := seedUser(t, svc, "other@acme.example")
	ext := oidc.ExternalUser{Provider: "github", Subject: "gh-9", Email: "owner@acme.example"}

	if err := svc.LinkExternalToUser(context.Background(), ext, owner.ID); err != nil {
		t.Fatalf("first link: %v", err)
	}
	if err := svc.LinkExternalToUser(context.Background(), ext, other.ID); !errors.Is(err, ErrLinkAlreadyOwned) {
		t.Fatalf("expected ErrLinkAlreadyOwned, got %v", err)
	}
}

func TestListAndUnlinkSSO(t *testing.T) {
	svc := newTestService(t, SignupInviteOnly)
	u := seedUser(t, svc, "bob@acme.example")
	other := seedUser(t, svc, "eve@acme.example")
	ext := oidc.ExternalUser{Provider: "google", Subject: "g-1", Email: "bob@acme.example"}
	if err := svc.LinkExternalToUser(context.Background(), ext, u.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	links, err := svc.ListSSOLinks(context.Background(), u.ID)
	if err != nil || len(links) != 1 || links[0].Provider != "google" {
		t.Fatalf("list links = %v, %v", links, err)
	}

	// A different user cannot unlink someone else's identity.
	if err := svc.UnlinkExternal(context.Background(), other.ID, ext.Provider, ext.Subject); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("cross-user unlink should be NotFound, got %v", err)
	}
	// The owner can.
	if err := svc.UnlinkExternal(context.Background(), u.ID, ext.Provider, ext.Subject); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	links, _ = svc.ListSSOLinks(context.Background(), u.ID)
	if len(links) != 0 {
		t.Fatalf("expected no links after unlink, got %v", links)
	}
}
