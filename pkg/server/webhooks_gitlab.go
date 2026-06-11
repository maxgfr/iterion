package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/store"
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

	// Conversational layer: dispatch a note (a /revi command or a reply to
	// the bot) to the note flow. See docs/forge-conversations.md.
	if r.Header.Get("X-Gitlab-Event") == gitlab.EventHeaderNote {
		s.handleGitLabNote(ctx, w, cfg, body, payloadHash, srcIP)
		return
	}

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
	runID, lerr := launch(ctx, botID, vars, p.CloneURL, p.SourceBranch, cfg.KeyOverrides, cfg.SecretOverrides)
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

// handleGitLabNote handles an inbound GitLab note (comment / reply) — the
// conversational path. A note triggers a run when it is a /revi command, the
// author is authorized (allowlist OR role-gate), and it is not the bot's own
// note (loop-guard). See docs/forge-conversations.md.
func (s *Server) handleGitLabNote(ctx context.Context, w http.ResponseWriter, cfg webhooks.Config, body []byte, payloadHash, srcIP string) {
	p, err := gitlab.ParseNote(body)
	if err != nil {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, p, err.Error())
		httpError(w, http.StatusBadRequest, "invalid note payload")
		return
	}
	filtered := func(reason string) {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusFiltered, payloadHash, srcIP, p, reason)
		writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusFiltered})
	}
	if !p.IsMergeRequestNote() ||
		!gitlab.MatchEvent(cfg.EventAllowlist, "note") ||
		!gitlab.MatchProject(cfg.ProjectAllowlist, p.ProjectPath) {
		filtered("out of scope (not an MR note / event / project)")
		return
	}
	// Trigger gate: an explicit /revi command. Reply-in-thread detection is a
	// follow-up (needs bot-thread tracking).
	cmd, cmdArgs := p.Command()
	if cmd != "revi" {
		filtered("no /revi trigger")
		return
	}
	botID := cfg.SelectBot()
	if botID == "" {
		botID = gitlabDefaultBot
	}
	if !cfg.AllowsBot(botID) {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, p, "bot not permitted by webhook scope")
		httpError(w, http.StatusForbidden, "bot %q not permitted by this webhook", botID)
		return
	}
	// Resolve the bot's forge token (honouring per-webhook secret overrides):
	// it authenticates the auth checks AND is the identity the bot posts as.
	token, terr := s.resolveForgeToken(ctx, cfg, botID)
	if terr != nil || token == "" {
		filtered("no forge token resolved (configure a forge_token binding)")
		return
	}
	api := gitlab.API{HTTP: s.httpClient, BaseURL: "https://" + hostFromURL(p.MRURL), Token: token}
	// Loop-guard: never act on the bot's own notes (else its reply re-fires).
	if bot, err := api.CurrentUser(ctx); err == nil && bot.ID == p.AuthorID {
		filtered("self note (loop-guard)")
		return
	}
	// Authorization: allowlist OR role-gate.
	ok, reason, aerr := gitlab.AuthorizeReplier(ctx, api, gitlab.ReplierAuth{
		AuthorID: p.AuthorID, AuthorUsername: p.AuthorUsername, ProjectID: p.ProjectID,
		Allowlist: cfg.AuthorizedRepliers, MinRole: cfg.MinReplierRole,
	})
	if aerr != nil {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusLaunchError, payloadHash, srcIP, p, "authz check: "+aerr.Error())
		httpError(w, http.StatusBadGateway, "authorization check failed")
		return
	}
	if !ok {
		filtered("replier not authorized: " + p.AuthorUsername)
		return
	}
	// Idempotency: one launch per note.
	idemKey := knowledge.ChecksumHex([]byte(fmt.Sprintf("%s|%s|%d|%s", cfg.TenantID, cfg.ID, p.ProjectID, p.SubjectID())))
	delivery := s.newGitLabNoteDelivery(cfg, p, webhooks.StatusAccepted, payloadHash, srcIP)
	delivery.IdempotencyKey = idemKey
	delivery.BotID = botID
	if s.webhookDeliveries != nil {
		if err := s.webhookDeliveries.Insert(ctx, delivery); err != nil {
			if errors.Is(err, webhooks.ErrDuplicate) {
				existing, _ := s.webhookDeliveries.GetByIdempotencyKey(ctx, idemKey)
				writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusDuplicate, "run_id": existing.RunID})
				return
			}
			httpError(w, http.StatusInternalServerError, "record delivery: %v", err)
			return
		}
	}
	// Launch: review-pr re-reviews the current MR state. The conversation vars
	// (discussion_id/trigger_note/replier) carry the thread context for the
	// converse bot + the forge.reply capability (A4/A5).
	vars := map[string]string{
		"pr_url":            p.MRURL,
		"base_ref":          p.TargetBranch,
		"scope_notes":       strings.TrimSpace(p.MRTitle + "\n\n" + p.MRDesc),
		"post_to_board":     "false",
		"pr_review_mode":    "summary",
		"conversation_mode": "reply",
		"discussion_id":     p.DiscussionID,
		"trigger_note":      p.NoteBody,
		"trigger_command":   cmd,
		"trigger_args":      cmdArgs,
		"replier":           p.AuthorUsername,
	}
	for k, v := range cfg.LaunchVars {
		vars[k] = v
	}
	launch := s.webhookLaunchBot
	if launch == nil {
		launch = s.realWebhookLaunchBot
	}
	runID, lerr := launch(ctx, botID, vars, p.CloneURL, p.SourceBranch, cfg.KeyOverrides, cfg.SecretOverrides)
	if lerr != nil {
		delivery.Status = webhooks.StatusLaunchError
		delivery.Error = lerr.Error()
		s.updateWebhookDelivery(ctx, delivery)
		httpError(w, http.StatusBadGateway, "launch failed: %v", lerr)
		return
	}
	now := time.Now().UTC()
	delivery.Status = webhooks.StatusLaunched
	delivery.RunID = runID
	delivery.LaunchedAt = &now
	s.updateWebhookDelivery(ctx, delivery)
	if s.logger != nil {
		s.logger.Info("webhooks: gitlab note %s!%d /%s by %s (authz=%s) launched %s run=%s", p.ProjectPath, p.MRIID, cmd, p.AuthorUsername, reason, botID, runID)
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]string{"status": webhooks.StatusLaunched, "run_id": runID, "delivery_id": delivery.ID})
}

// newGitLabNoteDelivery builds a delivery audit row for a note event.
func (s *Server) newGitLabNoteDelivery(cfg webhooks.Config, p gitlab.ParsedNote, status, payloadHash, srcIP string) webhooks.Delivery {
	return webhooks.Delivery{
		ID:          uuid.NewString(),
		TenantID:    cfg.TenantID,
		WebhookID:   cfg.ID,
		Provider:    cfg.Provider,
		EventKind:   "note",
		EventAction: "create",
		ProjectPath: p.ProjectPath,
		SubjectID:   p.SubjectID(),
		SubjectSHA:  p.HeadSHA,
		PayloadHash: payloadHash,
		Status:      status,
		SourceIP:    srcIP,
		ReceivedAt:  time.Now().UTC(),
	}
}

func (s *Server) recordNoteDelivery(ctx context.Context, cfg webhooks.Config, status, payloadHash, srcIP string, p gitlab.ParsedNote, errMsg string) {
	if s.webhookDeliveries == nil {
		return
	}
	d := s.newGitLabNoteDelivery(cfg, p, status, payloadHash, srcIP)
	d.IdempotencyKey = uuid.NewString()
	d.Error = errMsg
	_ = s.webhookDeliveries.Insert(ctx, d)
}

// resolveForgeToken resolves the bot's forge_token for the webhook's tenant,
// honouring its per-webhook secret override. Used at handler time for the
// auth checks (the per-run sealed bundle isn't available pre-launch).
func (s *Server) resolveForgeToken(ctx context.Context, cfg webhooks.Config, botID string) (string, error) {
	if s.genericSecrets == nil || s.sealer == nil {
		return "", nil
	}
	ctx = store.WithTenant(ctx, cfg.TenantID)
	res, err := secrets.ResolveGenericWithBindings(ctx, s.genericSecrets, s.botBindings, cfg.TenantID, "", botID, []string{"forge_token"}, cfg.SecretOverrides, s.sealer)
	if err != nil {
		return "", err
	}
	if r, ok := res["forge_token"]; ok {
		return string(r.Plaintext), nil
	}
	return "", nil
}

func hostFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
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
func (s *Server) realWebhookLaunchBot(ctx context.Context, botID string, vars map[string]string, repoURL, repoRef string, keyOverrides, secretOverrides map[string]string) (string, error) {
	if s.runs == nil {
		return "", errors.New("run service unavailable")
	}
	path, source, err := s.resolveBotSource(botID)
	if err != nil {
		return "", err
	}
	res, err := s.runs.Launch(ctx, runview.LaunchSpec{
		FilePath:        path,
		Source:          source,
		Vars:            vars,
		RepoURL:         repoURL,
		RepoRef:         repoRef,
		BotID:           botID,
		KeyOverrides:    keyOverrides,
		SecretOverrides: secretOverrides,
	})
	if err != nil {
		return "", err
	}
	return res.RunID, nil
}
