package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/SocialGouv/iterion/pkg/webhooks"
	"github.com/SocialGouv/iterion/pkg/webhooks/gitlab"
)

func glConfig() webhooks.Config {
	return webhooks.Config{ID: "w1", TenantID: "t1", Provider: webhooks.ProviderGitLab, Enabled: true, BotIDs: []string{"review-pr"}}
}

// gitlabCtx simulates what webhookAuth stamps before the handler runs.
func gitlabCtx(cfg webhooks.Config) context.Context {
	ctx := auth.WithIdentity(context.Background(), auth.Identity{UserID: "webhook:" + cfg.ID, TeamID: cfg.TenantID, Role: identity.RoleMember})
	ctx = store.WithIdentity(ctx, cfg.TenantID, "webhook:"+cfg.ID)
	return context.WithValue(ctx, webhookCtxKey{}, cfg)
}

func glReq(ctx context.Context, body, event string) *http.Request {
	r := httptest.NewRequest("POST", "/api/webhooks/gitlab/w1", strings.NewReader(body)).WithContext(ctx)
	if event != "" {
		r.Header.Set("X-Gitlab-Event", event)
	}
	r.SetPathValue("id", "w1")
	return r
}

const glOpenMR = `{
  "object_kind": "merge_request",
  "project": {"id": 42, "path_with_namespace": "acme/widgets", "git_http_url": "https://gitlab.com/acme/widgets.git"},
  "object_attributes": {"iid": 7, "action": "open", "source_branch": "feature/x", "target_branch": "main",
    "title": "Add X", "description": "desc", "url": "https://gitlab.com/acme/widgets/-/merge_requests/7",
    "last_commit": {"id": "sha1"}}
}`

func TestGitLabWebhook_HappyPath(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	var gotBot, gotURL, gotRef string
	var gotVars, gotKeyOverrides map[string]string
	s.webhookLaunchBot = func(_ context.Context, botID string, vars map[string]string, repoURL, repoRef string, keyOverrides map[string]string) (string, error) {
		calls++
		gotBot, gotVars, gotURL, gotRef, gotKeyOverrides = botID, vars, repoURL, repoRef, keyOverrides
		return "run-123", nil
	}
	cfg := glConfig()
	cfg.KeyOverrides = map[string]string{"anthropic": "key-abc"}
	w := httptest.NewRecorder()
	s.handleGitLabWebhook(w, glReq(gitlabCtx(cfg), glOpenMR, gitlab.EventHeaderMergeRequest))
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != webhooks.StatusLaunched || resp["run_id"] != "run-123" {
		t.Fatalf("resp: %v", resp)
	}
	if calls != 1 || gotBot != "review-pr" {
		t.Fatalf("launch calls=%d bot=%q", calls, gotBot)
	}
	if gotVars["pr_url"] != "https://gitlab.com/acme/widgets/-/merge_requests/7" || gotVars["base_ref"] != "main" || gotVars["post_to_board"] != "false" {
		t.Fatalf("vars: %v", gotVars)
	}
	if gotURL != "https://gitlab.com/acme/widgets.git" || gotRef != "feature/x" {
		t.Fatalf("repo: url=%q ref=%q", gotURL, gotRef)
	}
	if gotKeyOverrides["anthropic"] != "key-abc" {
		t.Fatalf("key overrides not threaded to launch: %v", gotKeyOverrides)
	}
	// delivery recorded as launched
	if list, _ := s.webhookDeliveries.ListByWebhook(context.Background(), "t1", "w1", 10); len(list) != 1 || list[0].Status != webhooks.StatusLaunched || list[0].RunID != "run-123" {
		t.Fatalf("delivery: %+v", list)
	}
}

func TestGitLabWebhook_Idempotent(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	s.webhookLaunchBot = func(_ context.Context, _ string, _ map[string]string, _, _ string, _ map[string]string) (string, error) {
		calls++
		return "run-123", nil
	}
	cfg := glConfig()
	w1 := httptest.NewRecorder()
	s.handleGitLabWebhook(w1, glReq(gitlabCtx(cfg), glOpenMR, gitlab.EventHeaderMergeRequest))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first: %d", w1.Code)
	}
	// replay same payload (same head sha) → duplicate, no second launch
	w2 := httptest.NewRecorder()
	s.handleGitLabWebhook(w2, glReq(gitlabCtx(cfg), glOpenMR, gitlab.EventHeaderMergeRequest))
	if w2.Code != http.StatusOK {
		t.Fatalf("replay code=%d body=%s", w2.Code, w2.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["status"] != webhooks.StatusDuplicate || resp["run_id"] != "run-123" {
		t.Fatalf("duplicate resp: %v", resp)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one launch, got %d", calls)
	}
}

func TestGitLabWebhook_FiltersAndRejects(t *testing.T) {
	s := newWebhookTestServer(t)
	var calls int
	s.webhookLaunchBot = func(_ context.Context, _ string, _ map[string]string, _, _ string, _ map[string]string) (string, error) {
		calls++
		return "r", nil
	}
	cfg := glConfig()

	// label-only update (no oldrev) → 200 filtered, no launch
	labelEdit := strings.Replace(glOpenMR, `"action": "open"`, `"action": "update"`, 1)
	w := httptest.NewRecorder()
	s.handleGitLabWebhook(w, glReq(gitlabCtx(cfg), labelEdit, gitlab.EventHeaderMergeRequest))
	if w.Code != http.StatusOK {
		t.Fatalf("label edit code=%d", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != webhooks.StatusFiltered {
		t.Fatalf("expected filtered, got %v", resp)
	}

	// project allowlist mismatch → filtered
	cfg2 := glConfig()
	cfg2.ProjectAllowlist = []string{"other/*"}
	w = httptest.NewRecorder()
	s.handleGitLabWebhook(w, glReq(gitlabCtx(cfg2), glOpenMR, gitlab.EventHeaderMergeRequest))
	json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || resp["status"] != webhooks.StatusFiltered {
		t.Fatalf("project mismatch: code=%d resp=%v", w.Code, resp)
	}

	// wrong event header → 400
	w = httptest.NewRecorder()
	s.handleGitLabWebhook(w, glReq(gitlabCtx(cfg), glOpenMR, "Push Hook"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong header code=%d", w.Code)
	}

	// malformed payload → 400
	w = httptest.NewRecorder()
	s.handleGitLabWebhook(w, glReq(gitlabCtx(cfg), `{bad`, gitlab.EventHeaderMergeRequest))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed code=%d", w.Code)
	}

	if calls != 0 {
		t.Fatalf("no launch should have happened, got %d", calls)
	}
}
