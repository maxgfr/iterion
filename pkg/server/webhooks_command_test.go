package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SocialGouv/iterion/pkg/webhooks"
	"github.com/SocialGouv/iterion/pkg/webhooks/gitlab"
)

// glNoteCmd is an MR note carrying a generic slash-command with args.
const glNoteFeaturly = `{
  "object_kind": "note",
  "project": {"id": 42, "path_with_namespace": "acme/widgets", "git_http_url": "https://gitlab.com/acme/widgets.git"},
  "user": {"username": "alice"},
  "object_attributes": {"id": 99, "note": "/featurly add an export endpoint", "noteable_type": "MergeRequest", "discussion_id": "d-1", "author_id": 1},
  "merge_request": {"iid": 7, "state": "opened", "source_branch": "feature/x", "target_branch": "main",
    "title": "Add X", "description": "desc", "url": "https://gitlab.com/acme/widgets/-/merge_requests/7",
    "last_commit": {"id": "headsha"}}
}`

func featurlyConfig() webhooks.Config {
	cfg := glConfig()
	cfg.BotIDs = []string{"review-pr", "feature-dev"}
	cfg.CommandMap = map[string][]webhooks.CommandRoute{
		"featurly": {{BotID: "feature-dev", Mode: "board", ArgsVar: "feature_prompt", Scope: "any"}},
	}
	return cfg
}

// TestGitLabNoteHook_GenericCommandLaunches pins the universal slash-command
// path: /featurly <spec> on an MR note resolves through the CommandMap to
// feature-dev, the args land in the route's args_var, and the bot launches.
func TestGitLabNoteHook_GenericCommandLaunches(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	var gotBot string
	var gotVars map[string]string
	s.webhookCommandGate = func(context.Context, webhooks.Config, gitlab.ParsedNote, webhooks.CommandRoute) (bool, string, error) {
		return true, "authorized", nil
	}
	s.webhookLaunchBot = func(_ context.Context, botID string, vars map[string]string, _, _, _ string, _, _ map[string]string) (string, error) {
		calls++
		gotBot, gotVars = botID, vars
		return "run-feat-1", nil
	}
	w := httptest.NewRecorder()
	s.handleGitLabWebhook(w, glNoteReq(gitlabCtx(featurlyConfig()), glNoteFeaturly))
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if calls != 1 || gotBot != "feature-dev" {
		t.Fatalf("launch: calls=%d bot=%q", calls, gotBot)
	}
	if gotVars["feature_prompt"] != "add an export endpoint" {
		t.Fatalf("args should land in feature_prompt: %v", gotVars["feature_prompt"])
	}
	if gotVars["scope_notes"] == "" || gotVars["pr_url"] == "" {
		t.Fatalf("PR context vars missing: %v", gotVars)
	}
}

// TestGitLabNoteHook_UnknownCommandFiltered: a command no bot claims (on a
// non-wildcard webhook) is filtered with 200, never launched, never 4xx.
func TestGitLabNoteHook_UnknownCommandFiltered(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	s.webhookCommandGate = func(context.Context, webhooks.Config, gitlab.ParsedNote, webhooks.CommandRoute) (bool, string, error) {
		return true, "ok", nil
	}
	s.webhookLaunchBot = func(context.Context, string, map[string]string, string, string, string, map[string]string, map[string]string) (string, error) {
		calls++
		return "x", nil
	}
	body := `{"object_kind":"note","project":{"id":42,"path_with_namespace":"acme/widgets"},"user":{"username":"alice"},"object_attributes":{"id":99,"note":"/bogus do something","noteable_type":"MergeRequest","discussion_id":"d-1","author_id":1},"merge_request":{"iid":7,"state":"opened","target_branch":"main","title":"X","url":"https://gitlab.com/acme/widgets/-/merge_requests/7"}}`
	w := httptest.NewRecorder()
	s.handleGitLabWebhook(w, glNoteReq(gitlabCtx(featurlyConfig()), body))
	if w.Code != http.StatusOK {
		t.Fatalf("unknown command should be filtered 200, got %d", w.Code)
	}
	if calls != 0 {
		t.Fatalf("unknown command must not launch, calls=%d", calls)
	}
}

// TestGitLabNoteHook_CommandUnauthorizedFiltered: a denied replier filters
// (200) without launching.
func TestGitLabNoteHook_CommandUnauthorizedFiltered(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	s.webhookCommandGate = func(context.Context, webhooks.Config, gitlab.ParsedNote, webhooks.CommandRoute) (bool, string, error) {
		return false, "replier not authorized", nil
	}
	s.webhookLaunchBot = func(context.Context, string, map[string]string, string, string, string, map[string]string, map[string]string) (string, error) {
		calls++
		return "x", nil
	}
	w := httptest.NewRecorder()
	s.handleGitLabWebhook(w, glNoteReq(gitlabCtx(featurlyConfig()), glNoteFeaturly))
	if w.Code != http.StatusOK {
		t.Fatalf("unauthorized should be filtered 200, got %d", w.Code)
	}
	if calls != 0 {
		t.Fatalf("unauthorized must not launch, calls=%d", calls)
	}
}
