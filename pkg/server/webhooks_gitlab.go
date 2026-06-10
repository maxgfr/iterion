package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/webhooks"
	"github.com/SocialGouv/iterion/pkg/webhooks/gitlab"
)

// gitlabDefaultBot is the bot V1 pins for an MR review when the webhook
// scope is a wildcard or otherwise ambiguous.
const gitlabDefaultBot = "review-pr"

// maxWebhookBodyBytes caps the inbound payload we read.
const maxWebhookBodyBytes = 5 << 20

// registerGitLabWebhookRoute wires the inbound GitLab MR delivery
// endpoint behind webhookAuth.
func (s *Server) registerGitLabWebhookRoute() {
	s.mux.Handle("POST /api/webhooks/gitlab/{id}", s.webhookAuth(webhooks.ProviderGitLab, http.HandlerFunc(s.handleGitLabWebhook)))
}

// handleGitLabWebhook handles a verified inbound GitLab MR webhook. Auth,
// rate-limit, quota, suspend-check and tenant stamping are already done
// by webhookAuth; the config is on ctx.
func (s *Server) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, ok := webhookConfigFromContext(ctx)
	if !ok {
		httpError(w, http.StatusInternalServerError, "webhook context missing")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes))
	if err != nil {
		httpError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	payloadHash := knowledge.ChecksumHex(body)
	srcIP := s.clientIP(r)

	if r.Header.Get("X-Gitlab-Event") != gitlab.EventHeaderMergeRequest {
		s.recordWebhookDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, gitlab.Parsed{}, "unsupported X-Gitlab-Event")
		httpError(w, http.StatusBadRequest, "unsupported event (merge_request only)")
		return
	}
	p, err := gitlab.ParseMergeRequest(body)
	if err != nil {
		s.recordWebhookDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, gitlab.Parsed{}, err.Error())
		httpError(w, http.StatusBadRequest, "invalid merge_request payload")
		return
	}

	// Filter: only review on open/reopen/new-push, allowed event + project.
	// A filtered delivery returns 200 (a 4xx would make GitLab disable the
	// webhook after repeated metadata-only edits).
	if !p.IsReviewable() ||
		!gitlab.MatchEvent(cfg.EventAllowlist, "merge_request") ||
		!gitlab.MatchProject(cfg.ProjectAllowlist, p.ProjectPath) {
		s.recordWebhookDelivery(ctx, cfg, webhooks.StatusFiltered, payloadHash, srcIP, p, "")
		writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusFiltered})
		return
	}

	// Bot selection (V1 pins Revi for wildcard/ambiguous).
	botID := cfg.SelectBot()
	if botID == "" {
		botID = gitlabDefaultBot
	}
	if !cfg.AllowsBot(botID) {
		s.recordWebhookDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, p, "bot not permitted by webhook scope")
		httpError(w, http.StatusForbidden, "bot %q not permitted by this webhook", botID)
		return
	}

	// Idempotency: one launch per (tenant, webhook, project, MR, head sha).
	idemKey := knowledge.ChecksumHex([]byte(fmt.Sprintf("%s|%s|%d|%d|%s", cfg.TenantID, cfg.ID, p.ProjectID, p.MRIID, p.HeadSHA)))
	delivery := newGitLabDelivery(cfg, p, webhooks.StatusAccepted, payloadHash, srcIP)
	delivery.IdempotencyKey = idemKey
	delivery.BotID = botID
	if s.webhookDeliveries != nil {
		if err := s.webhookDeliveries.Insert(ctx, delivery); err != nil {
			if errors.Is(err, webhooks.ErrDuplicate) {
				existing, _ := s.webhookDeliveries.GetByIdempotencyKey(ctx, idemKey)
				writeJSONStatus(w, http.StatusOK, map[string]string{
					"status": webhooks.StatusDuplicate, "run_id": existing.RunID, "delivery_id": existing.ID,
				})
				return
			}
			httpError(w, http.StatusInternalServerError, "record delivery: %v", err)
			return
		}
	}

	// Build the launch vars. cfg.LaunchVars (operator-pinned) override.
	vars := map[string]string{
		"pr_url":         p.MRURL,
		"base_ref":       p.TargetBranch,
		"scope_notes":    strings.TrimSpace(p.Title + "\n\n" + p.Description),
		"post_to_board":  "false",
		"pr_review_mode": "summary",
	}
	for k, v := range cfg.LaunchVars {
		vars[k] = v
	}

	launch := s.webhookLaunchBot
	if launch == nil {
		launch = s.realWebhookLaunchBot
	}
	runID, lerr := launch(ctx, botID, vars, p.CloneURL, p.SourceBranch)
	if lerr != nil {
		delivery.Status = webhooks.StatusLaunchError
		delivery.Error = lerr.Error()
		s.updateWebhookDelivery(ctx, delivery)
		httpError(w, http.StatusBadGateway, "launch failed: %v", lerr)
		return
	}
	launchedAt := time.Now().UTC()
	delivery.Status = webhooks.StatusLaunched
	delivery.RunID = runID
	delivery.LaunchedAt = &launchedAt
	s.updateWebhookDelivery(ctx, delivery)

	if s.logger != nil {
		s.logger.Info("webhooks: gitlab MR %s/%s!%d launched %s run=%s", p.ProjectPath, botID, p.MRIID, botID, runID)
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]string{
		"status": webhooks.StatusLaunched, "run_id": runID, "delivery_id": delivery.ID,
	})
}

// newGitLabDelivery builds the common fields of a delivery audit row;
// callers layer the idempotency key + outcome-specific fields on top.
func newGitLabDelivery(cfg webhooks.Config, p gitlab.Parsed, status, payloadHash, srcIP string) webhooks.Delivery {
	subject := ""
	if p.MRIID != 0 {
		subject = p.SubjectID()
	}
	return webhooks.Delivery{
		ID:          uuid.NewString(),
		TenantID:    cfg.TenantID,
		WebhookID:   cfg.ID,
		Provider:    cfg.Provider,
		EventKind:   "merge_request",
		EventAction: p.Action,
		ProjectPath: p.ProjectPath,
		SubjectID:   subject,
		SubjectSHA:  p.HeadSHA,
		PayloadHash: payloadHash,
		Status:      status,
		SourceIP:    srcIP,
		ReceivedAt:  time.Now().UTC(),
	}
}

// recordWebhookDelivery inserts a terminal (non-launched) audit row with
// a unique idempotency key so it never collides with the dedup key.
// Best-effort.
func (s *Server) recordWebhookDelivery(ctx context.Context, cfg webhooks.Config, status, payloadHash, srcIP string, p gitlab.Parsed, errMsg string) {
	if s.webhookDeliveries == nil {
		return
	}
	d := newGitLabDelivery(cfg, p, status, payloadHash, srcIP)
	d.IdempotencyKey = uuid.NewString()
	d.Error = errMsg
	_ = s.webhookDeliveries.Insert(ctx, d)
}

func (s *Server) updateWebhookDelivery(ctx context.Context, d webhooks.Delivery) {
	if s.webhookDeliveries == nil {
		return
	}
	_ = s.webhookDeliveries.Update(ctx, d)
}

// resolveBotSource loads a bot's workflow source by bot id.
func (s *Server) resolveBotSource(botID string) (path, source string, err error) {
	path, err = botregistry.ResolveBotPath(botID, s.effectivePaths())
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read bot %q: %w", botID, err)
	}
	return path, string(b), nil
}

// realWebhookLaunchBot is the production launch path for an inbound
// webhook: resolve the bot's source and submit it through the run
// service (which, in cloud mode, routes to the publisher).
func (s *Server) realWebhookLaunchBot(ctx context.Context, botID string, vars map[string]string, repoURL, repoRef string) (string, error) {
	if s.runs == nil {
		return "", errors.New("run service unavailable")
	}
	path, source, err := s.resolveBotSource(botID)
	if err != nil {
		return "", err
	}
	res, err := s.runs.Launch(ctx, runview.LaunchSpec{
		FilePath: path,
		Source:   source,
		Vars:     vars,
		RepoURL:  repoURL,
		RepoRef:  repoRef,
		BotID:    botID,
	})
	if err != nil {
		return "", err
	}
	return res.RunID, nil
}
