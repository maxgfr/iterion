package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/webhooks"
)

func newWebhookTestServer(t *testing.T) *Server {
	t.Helper()
	s := newOrgTestServer(t)
	s.webhookConfigs = webhooks.NewMemoryConfigStore()
	s.webhookDeliveries = webhooks.NewMemoryDeliveryStore()
	s.webhookCounter = webhooks.NewMemoryCounter()
	s.authLimiter = newAuthRateLimiter()
	return s
}

func whReq(ctx context.Context, method, path, body, teamID, webhookID string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	r = r.WithContext(ctx)
	if teamID != "" {
		r.SetPathValue("id", teamID)
	}
	if webhookID != "" {
		r.SetPathValue("webhook_id", webhookID)
	}
	return r
}

func TestHandleCreateWebhook_TokenOnceAndScope(t *testing.T) {
	s := newWebhookTestServer(t)
	ctx := superAdminCtx()

	// missing bot scope → 400
	w := httptest.NewRecorder()
	s.handleCreateWebhook(w, whReq(ctx, "POST", "/api/teams/t1/webhooks", `{"name":"gl"}`, "t1", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing bot scope: code=%d body=%s", w.Code, w.Body.String())
	}

	// bare "*" without wildcard_bots → 400
	w = httptest.NewRecorder()
	s.handleCreateWebhook(w, whReq(ctx, "POST", "/api/teams/t1/webhooks", `{"name":"gl","bot_ids":["*"]}`, "t1", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bare wildcard: code=%d", w.Code)
	}

	// valid create → 201, token present once + last4
	w = httptest.NewRecorder()
	s.handleCreateWebhook(w, whReq(ctx, "POST", "/api/teams/t1/webhooks", `{"name":"gl","bot_ids":["review-pr"]}`, "t1", ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}
	var created webhookWithToken
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || !strings.HasPrefix(created.Token, webhooks.TokenPrefix) {
		t.Fatalf("token not returned once: %q", created.Token)
	}
	if created.Config.TokenLast4 == "" || created.Config.TenantID != "t1" {
		t.Fatalf("bad config: %+v", created.Config)
	}

	// GET never leaks the token (token_hash is json:"-").
	w = httptest.NewRecorder()
	s.handleGetWebhook(w, whReq(ctx, "GET", "/api/teams/t1/webhooks/"+created.Config.ID, "", "t1", created.Config.ID))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), created.Token) || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatalf("GET leaked token material: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateWebhook_WildcardRequiresFlag(t *testing.T) {
	s := newWebhookTestServer(t)
	ctx := superAdminCtx()
	w := httptest.NewRecorder()
	s.handleCreateWebhook(w, whReq(ctx, "POST", "/api/teams/t1/webhooks", `{"name":"any","wildcard_bots":true}`, "t1", ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("wildcard create: code=%d body=%s", w.Code, w.Body.String())
	}
	var created webhookWithToken
	json.Unmarshal(w.Body.Bytes(), &created)
	if !created.Config.WildcardBots || len(created.Config.BotIDs) != 1 || created.Config.BotIDs[0] != "*" {
		t.Fatalf("wildcard not normalised: %+v", created.Config)
	}
	if !created.Config.AllowsBot("anything") {
		t.Fatal("wildcard should allow any bot")
	}
}

func TestHandleRotateWebhook(t *testing.T) {
	s := newWebhookTestServer(t)
	ctx := superAdminCtx()
	// seed a config with a known token
	pt, hash, last4, fp, _ := webhooks.MintToken()
	cfg := webhooks.Config{ID: "w1", TenantID: "t1", Provider: webhooks.ProviderGitLab, Enabled: true,
		TokenHash: hash, TokenLast4: last4, Fingerprint: fp, BotIDs: []string{"review-pr"}, CreatedAt: time.Now()}
	if err := s.webhookConfigs.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleRotateWebhook(w, whReq(ctx, "POST", "/api/teams/t1/webhooks/w1/rotate", "", "t1", "w1"))
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: code=%d", w.Code)
	}
	var rotated webhookWithToken
	json.Unmarshal(w.Body.Bytes(), &rotated)
	if rotated.Token == "" || rotated.Token == pt {
		t.Fatalf("rotate should mint a fresh token, got %q", rotated.Token)
	}
	stored, _ := s.webhookConfigs.Get(ctx, "w1")
	if webhooks.VerifyToken(pt, stored.TokenHash) {
		t.Fatal("old token must no longer verify after rotate")
	}
	if !webhooks.VerifyToken(rotated.Token, stored.TokenHash) {
		t.Fatal("new token must verify after rotate")
	}
}

// webhookAuth admission: drive the middleware with a recording next.
func runWebhookAuth(s *Server, cfgID, token string) (status int, ran bool, tenant, gotCfg string) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		if c, ok := webhookConfigFromContext(r.Context()); ok {
			gotCfg = c.ID
		}
		if id, ok := auth.FromContext(r.Context()); ok {
			tenant = id.TeamID
		}
		w.WriteHeader(http.StatusOK)
	})
	h := s.webhookAuth(webhooks.ProviderGitLab, next)
	r := httptest.NewRequest("POST", "/api/webhooks/gitlab/"+cfgID, strings.NewReader("{}"))
	r.SetPathValue("id", cfgID)
	if token != "" {
		r.Header.Set("X-Gitlab-Token", token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, ran, tenant, gotCfg
}

func TestWebhookAuth_Admission(t *testing.T) {
	s := newWebhookTestServer(t)
	pt, hash, last4, fp, _ := webhooks.MintToken()
	cfg := webhooks.Config{ID: "w1", TenantID: "t1", Provider: webhooks.ProviderGitLab, Enabled: true,
		TokenHash: hash, TokenLast4: last4, Fingerprint: fp, BotIDs: []string{"review-pr"},
		RateLimit: webhooks.Rate{Rate: 100, Burst: 100}, CreatedAt: time.Now()}
	if err := s.webhookConfigs.Create(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	// valid token → next runs, tenant + config stamped
	if code, ran, tenant, gotCfg := runWebhookAuth(s, "w1", pt); code != http.StatusOK || !ran || tenant != "t1" || gotCfg != "w1" {
		t.Fatalf("valid: code=%d ran=%v tenant=%q cfg=%q", code, ran, tenant, gotCfg)
	}
	// bad token → 401, next not run
	if code, ran, _, _ := runWebhookAuth(s, "w1", pt+"x"); code != http.StatusUnauthorized || ran {
		t.Fatalf("bad token: code=%d ran=%v", code, ran)
	}
	// missing token → 401
	if code, _, _, _ := runWebhookAuth(s, "w1", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing token: code=%d", code)
	}
	// unknown id → 401 (not probeable)
	if code, _, _, _ := runWebhookAuth(s, "ghost", pt); code != http.StatusUnauthorized {
		t.Fatalf("unknown id: code=%d", code)
	}
}

func TestWebhookAuth_DisabledAndRateLimitAndQuota(t *testing.T) {
	s := newWebhookTestServer(t)
	ctx := context.Background()

	// disabled → 410
	pt, hash, l4, fp, _ := webhooks.MintToken()
	s.webhookConfigs.Create(ctx, webhooks.Config{ID: "off", TenantID: "t1", Provider: webhooks.ProviderGitLab,
		Enabled: false, TokenHash: hash, TokenLast4: l4, Fingerprint: fp, BotIDs: []string{"x"}, RateLimit: webhooks.Rate{Rate: 100, Burst: 100}})
	if code, ran, _, _ := runWebhookAuth(s, "off", pt); code != http.StatusGone || ran {
		t.Fatalf("disabled: code=%d ran=%v", code, ran)
	}

	// rate limit: burst 1 → first ok, second 429
	pt2, h2, l42, fp2, _ := webhooks.MintToken()
	s.webhookConfigs.Create(ctx, webhooks.Config{ID: "rl", TenantID: "t1", Provider: webhooks.ProviderGitLab,
		Enabled: true, TokenHash: h2, TokenLast4: l42, Fingerprint: fp2, BotIDs: []string{"x"}, RateLimit: webhooks.Rate{Rate: 0.0001, Burst: 1}})
	if code, _, _, _ := runWebhookAuth(s, "rl", pt2); code != http.StatusOK {
		t.Fatalf("rl first: code=%d", code)
	}
	if code, _, _, _ := runWebhookAuth(s, "rl", pt2); code != http.StatusTooManyRequests {
		t.Fatalf("rl second should be 429: code=%d", code)
	}

	// monthly quota: per-webhook limit 1, high rate so RL doesn't trip
	pt3, h3, l43, fp3, _ := webhooks.MintToken()
	s.webhookConfigs.Create(ctx, webhooks.Config{ID: "mq", TenantID: "t2", Provider: webhooks.ProviderGitLab,
		Enabled: true, TokenHash: h3, TokenLast4: l43, Fingerprint: fp3, BotIDs: []string{"x"},
		RateLimit: webhooks.Rate{Rate: 100, Burst: 100}, MonthlyCallLimit: 1})
	if code, _, _, _ := runWebhookAuth(s, "mq", pt3); code != http.StatusOK {
		t.Fatalf("mq first: code=%d", code)
	}
	if code, _, _, _ := runWebhookAuth(s, "mq", pt3); code != http.StatusTooManyRequests {
		t.Fatalf("mq second should be 429 (monthly quota): code=%d", code)
	}
}
