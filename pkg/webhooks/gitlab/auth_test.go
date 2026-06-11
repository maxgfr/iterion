package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInAllowlist(t *testing.T) {
	allow := []string{"@alice", "bob", "42"}
	cases := []struct {
		user string
		id   int64
		want bool
	}{
		{"alice", 1, true},  // @alice
		{"ALICE", 1, true},  // case-insensitive
		{"bob", 2, true},    // bare username
		{"carol", 42, true}, // by id
		{"carol", 7, false}, // neither
	}
	for _, c := range cases {
		if got := InAllowlist(allow, c.user, c.id); got != c.want {
			t.Errorf("InAllowlist(%q,%d) = %v; want %v", c.user, c.id, got, c.want)
		}
	}
}

func TestAuthorizeReplier(t *testing.T) {
	// Fake GitLab: dev (id 30) is a Developer member; guest (id 10) a Guest.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/194/members/all/30":
			w.Write([]byte(`{"access_level":30}`))
		case "/api/v4/projects/194/members/all/10":
			w.Write([]byte(`{"access_level":10}`))
		default:
			w.WriteHeader(http.StatusNotFound) // non-member
		}
	}))
	defer srv.Close()
	api := API{HTTP: srv.Client(), BaseURL: srv.URL, Token: "t"}

	// (a) allowlist hit → authorized without an API call.
	ok, why, err := AuthorizeReplier(context.Background(), API{BaseURL: "http://unused"}, ReplierAuth{
		AuthorID: 99, AuthorUsername: "ext", ProjectID: 194, Allowlist: []string{"ext"}, MinRole: "maintainer",
	})
	if err != nil || !ok || why != "allowlist" {
		t.Fatalf("allowlist: ok=%v why=%q err=%v", ok, why, err)
	}

	// (b) role-gate: Developer >= developer floor → authorized.
	ok, why, _ = AuthorizeReplier(context.Background(), api, ReplierAuth{
		AuthorID: 30, AuthorUsername: "dev", ProjectID: 194, MinRole: "developer",
	})
	if !ok || why != "role" {
		t.Fatalf("developer role: ok=%v why=%q", ok, why)
	}

	// (c) Guest < developer floor, not in allowlist → denied.
	ok, _, _ = AuthorizeReplier(context.Background(), api, ReplierAuth{
		AuthorID: 10, AuthorUsername: "guest", ProjectID: 194, MinRole: "developer",
	})
	if ok {
		t.Fatal("guest should be denied")
	}

	// (d) non-member, not in allowlist → denied.
	ok, _, _ = AuthorizeReplier(context.Background(), api, ReplierAuth{
		AuthorID: 7, AuthorUsername: "stranger", ProjectID: 194, MinRole: "developer",
	})
	if ok {
		t.Fatal("non-member should be denied")
	}
}
