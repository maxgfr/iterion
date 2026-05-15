package tracker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// fakeGH is a Command stub that returns canned JSON per gh subcommand.
type fakeGH struct {
	mu      sync.Mutex
	listOut []byte
	apiOut  []byte // response for `gh api repos/<...>/issues...` (RefreshStates)
	calls   [][]string
	failNum int
}

func (f *fakeGH) cmd(_ context.Context, args []string, _ []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) == 0 {
		return nil, errors.New("no args")
	}
	switch {
	case args[0] == "issue" && args[1] == "list":
		return f.listOut, nil
	case args[0] == "api":
		return f.apiOut, nil
	case args[0] == "issue" && (args[1] == "edit" || args[1] == "comment"):
		if f.failNum != 0 {
			var n int
			_, _ = fmt.Sscanf(args[2], "%d", &n)
			if n == f.failNum {
				return nil, errors.New("simulated failure")
			}
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unhandled args: %v", args)
}

func newGHAdapter(t *testing.T, fake *fakeGH, mapping map[string]tracker.LabelSelector) *tracker.GitHubAdapter {
	t.Helper()
	a, err := tracker.NewGitHub(tracker.GitHubOptions{
		Repo:         "owner/repo",
		StateMapping: mapping,
		Command:      fake.cmd,
	})
	if err != nil {
		t.Fatalf("NewGitHub: %v", err)
	}
	return a
}

func TestGitHubListCandidates(t *testing.T) {
	fake := &fakeGH{
		listOut: mustJSON([]map[string]any{
			{
				"number":    42,
				"title":     "fix the bug",
				"body":      "body",
				"state":     "open",
				"labels":    []map[string]string{{"name": "ready"}},
				"createdAt": "2026-05-01T00:00:00Z",
				"updatedAt": "2026-05-01T00:00:00Z",
				"url":       "https://github.com/owner/repo/issues/42",
			},
			{
				// no matching label → filtered out
				"number":    99,
				"title":     "untriaged",
				"state":     "open",
				"labels":    []map[string]string{{"name": "noise"}},
				"createdAt": "2026-05-02T00:00:00Z",
				"updatedAt": "2026-05-02T00:00:00Z",
				"url":       "https://github.com/owner/repo/issues/99",
			},
		}),
	}
	a := newGHAdapter(t, fake, map[string]tracker.LabelSelector{
		"ready": {LabelsInclude: []string{"ready"}},
	})
	got, err := a.ListCandidates(context.Background())
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 issue, got %d", len(got))
	}
	if got[0].ID != "github:owner/repo#42" {
		t.Fatalf("ID: %s", got[0].ID)
	}
	if got[0].WorkflowState != "ready" {
		t.Fatalf("state: %s", got[0].WorkflowState)
	}
	if got[0].Metadata["url"] != "https://github.com/owner/repo/issues/42" {
		t.Fatalf("url metadata: %s", got[0].Metadata["url"])
	}
	if !strings.Contains(strings.Join(fake.calls[0], " "), "-label:iterion-claimed") {
		t.Fatalf("expected default claim filter in search: %v", fake.calls[0])
	}
}

func TestGitHubResolveStateOrder(t *testing.T) {
	fake := &fakeGH{
		listOut: mustJSON([]map[string]any{
			{"number": 1, "labels": []map[string]string{{"name": "ready"}, {"name": "claimed"}}, "title": "x", "createdAt": "2026-05-01T00:00:00Z", "updatedAt": "2026-05-01T00:00:00Z"},
		}),
	}
	a := newGHAdapter(t, fake, map[string]tracker.LabelSelector{
		"in_progress": {LabelsInclude: []string{"claimed"}},
		"ready":       {LabelsInclude: []string{"ready"}, LabelsExclude: []string{"claimed"}},
	})
	got, _ := a.ListCandidates(context.Background())
	// in_progress matches because the issue has "claimed". Sorted state names: "in_progress" < "ready".
	if got[0].WorkflowState != "in_progress" {
		t.Fatalf("state: %s", got[0].WorkflowState)
	}
}

func TestGitHubRefreshStates(t *testing.T) {
	// The adapter now batches RefreshStates into a single `gh api` call
	// and filters locally. The fake returns a REST-shaped list.
	fake := &fakeGH{
		apiOut: mustJSON([]map[string]any{
			{
				"number":     7,
				"title":      "x",
				"labels":     []map[string]string{{"name": "ready"}},
				"state":      "open",
				"created_at": "2026-05-01T00:00:00Z",
				"updated_at": "2026-05-01T00:00:00Z",
			},
			{
				// Same repo, different issue, not in our wanted set.
				"number":     11,
				"title":      "stranger",
				"labels":     []map[string]string{{"name": "ready"}},
				"state":      "open",
				"created_at": "2026-05-01T00:00:00Z",
				"updated_at": "2026-05-01T00:00:00Z",
			},
		}),
	}
	a := newGHAdapter(t, fake, map[string]tracker.LabelSelector{
		"ready": {LabelsInclude: []string{"ready"}},
	})
	got, err := a.RefreshStates(context.Background(), []string{"github:owner/repo#7", "github:owner/repo#9999", "bogus"})
	if err != nil {
		t.Fatalf("RefreshStates: %v", err)
	}
	if got["github:owner/repo#7"] != "ready" {
		t.Fatalf("state for #7: %s", got["github:owner/repo#7"])
	}
	if _, ok := got["github:owner/repo#9999"]; ok {
		t.Fatal("missing ID should be omitted")
	}
	if _, ok := got["github:owner/repo#11"]; ok {
		t.Fatal("issue outside the wanted set should be filtered out")
	}
	// One API call covers any number of IDs.
	apiCalls := 0
	for _, c := range fake.calls {
		if len(c) > 0 && c[0] == "api" {
			apiCalls++
		}
	}
	if apiCalls != 1 {
		t.Fatalf("expected 1 `gh api` call (batch), got %d", apiCalls)
	}
}

func TestGitHubClaimAndRelease(t *testing.T) {
	fake := &fakeGH{}
	a := newGHAdapter(t, fake, nil)
	if err := a.Claim(context.Background(), "github:owner/repo#5", "h-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := a.Release(context.Background(), "github:owner/repo#5", "h-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Last two calls should be edit --add-label, edit --remove-label.
	if !contains(fake.calls[0], "--add-label") || !contains(fake.calls[1], "--remove-label") {
		t.Fatalf("unexpected calls: %v", fake.calls)
	}
}

func TestGitHubUpdateStateMissingMapping(t *testing.T) {
	fake := &fakeGH{}
	a := newGHAdapter(t, fake, map[string]tracker.LabelSelector{
		"ready": {LabelsInclude: []string{"ready"}},
	})
	err := a.UpdateState(context.Background(), "github:owner/repo#1", "noplace")
	if !errors.Is(err, tracker.ErrTransitionRejected) {
		t.Fatalf("want ErrTransitionRejected, got %v", err)
	}
}

func TestGitHubInvalidID(t *testing.T) {
	fake := &fakeGH{}
	a := newGHAdapter(t, fake, nil)
	if err := a.Claim(context.Background(), "native:abc", "h"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGitHubCmdFailureBubblesUp(t *testing.T) {
	fake := &fakeGH{failNum: 8}
	a := newGHAdapter(t, fake, nil)
	err := a.Claim(context.Background(), "github:owner/repo#8", "h")
	if err == nil || !strings.Contains(err.Error(), "simulated failure") {
		t.Fatalf("expected wrapped failure, got %v", err)
	}
}

// Compile-time assertion: *GitHubAdapter satisfies tracker.Tracker.
var _ tracker.Tracker = (*tracker.GitHubAdapter)(nil)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
