package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/forge"
)

func TestWhoAmI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v4/user" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "username": "alice", "email": "a@x.io"})
	}))
	defer srv.Close()

	id, err := New(srv.Client(), srv.URL, "tok-123").WhoAmI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id.Login != "alice" || id.ID != "7" || id.Kind != "user" {
		t.Errorf("identity = %+v", id)
	}
}

// TestCreateHook_BooleanBodyShape is the deterministic stand-in for risk #1
// in the plan: GitLab's POST /hooks takes BOOLEAN event fields, not an
// events array. This pins the exact request body so a regression in the
// translation (event_map / events.go) fails here, not silently on a live
// GitLab.
func TestCreateHook_BooleanBodyShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.URL.EscapedPath(); !strings.HasSuffix(got, "/projects/group%2Fapi/hooks") {
			t.Errorf("escaped path = %q, want namespaced project id", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "url": gotBody["url"], "merge_requests_events": true, "note_events": true,
		})
	}))
	defer srv.Close()

	h, err := New(srv.Client(), srv.URL, "tok").CreateHook(context.Background(), "group/api", forge.HookSpec{
		URL:    "https://iterion.example.com/api/webhooks/gitlab/wh1",
		Secret: "iwh_secret",
		Events: []string{"merge_request", "note"},
		Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The boolean translation is the crux.
	if gotBody["merge_requests_events"] != true {
		t.Errorf("merge_requests_events = %v, want true", gotBody["merge_requests_events"])
	}
	if gotBody["note_events"] != true {
		t.Errorf("note_events = %v, want true", gotBody["note_events"])
	}
	if gotBody["push_events"] != false {
		t.Errorf("push_events = %v, want false", gotBody["push_events"])
	}
	if gotBody["enable_ssl_verification"] != true {
		t.Errorf("enable_ssl_verification = %v, want true", gotBody["enable_ssl_verification"])
	}
	if gotBody["token"] != "iwh_secret" {
		t.Errorf("token = %v, want the iwh_ secret", gotBody["token"])
	}
	if h.ID != "42" {
		t.Errorf("hook id = %q, want 42", h.ID)
	}
	if len(h.Events) != 2 {
		t.Errorf("returned events = %v", h.Events)
	}
}

func TestCreateHook_SingleEventOmitsOther(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "url": gotBody["url"], "note_events": true})
	}))
	defer srv.Close()

	_, err := New(srv.Client(), srv.URL, "tok").CreateHook(context.Background(), "g/p", forge.HookSpec{
		URL: "u", Secret: "s", Events: []string{"note"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["note_events"] != true || gotBody["merge_requests_events"] != false {
		t.Errorf("single-event body wrong: %v", gotBody)
	}
}

func TestGetHook_MatchByURL(t *testing.T) {
	const wantURL = "https://iterion.example.com/api/webhooks/gitlab/wh1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "url": "https://other/hook", "merge_requests_events": true},
			{"id": 2, "url": wantURL, "merge_requests_events": true, "note_events": true},
		})
	}))
	defer srv.Close()

	c := New(srv.Client(), srv.URL, "tok")
	got, err := c.GetHook(context.Background(), "g/p", wantURL)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "2" {
		t.Fatalf("got = %+v, want hook id 2", got)
	}

	none, err := c.GetHook(context.Background(), "g/p", "https://nomatch")
	if err != nil || none != nil {
		t.Errorf("expected no match, got %+v err %v", none, err)
	}
}

func TestDeleteHook_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	err := New(srv.Client(), srv.URL, "tok").DeleteHook(context.Background(), "g/p", "9")
	if !errors.Is(err, forge.ErrHookNotFound) {
		t.Errorf("delete 404 = %v, want ErrHookNotFound", err)
	}
}

func TestCreateHook_403IsForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := New(srv.Client(), srv.URL, "tok").CreateHook(context.Background(), "g/p", forge.HookSpec{URL: "u", Events: []string{"note"}})
	if !errors.Is(err, forge.ErrForbidden) {
		t.Errorf("create 403 = %v, want ErrForbidden", err)
	}
}

func TestListRepos_FiltersAndMaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("min_access_level") != "40" {
			t.Errorf("min_access_level = %q, want 40", r.URL.Query().Get("min_access_level"))
		}
		if r.URL.Query().Get("membership") != "true" {
			t.Errorf("membership = %q", r.URL.Query().Get("membership"))
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "path_with_namespace": "group/api", "visibility": "private", "default_branch": "main", "web_url": "https://gl/group/api"},
		})
	}))
	defer srv.Close()

	repos, err := New(srv.Client(), srv.URL, "tok").ListRepos(context.Background(), forge.RepoQuery{Search: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("repos = %d", len(repos))
	}
	r := repos[0]
	if r.FullName != "group/api" || !r.Private || r.DefaultBranch != "main" || !r.CanAdmin {
		t.Errorf("repo mapping wrong: %+v", r)
	}
}

// compile-time assertion that AdminClient satisfies forge.Admin.
var _ forge.Admin = (*AdminClient)(nil)
