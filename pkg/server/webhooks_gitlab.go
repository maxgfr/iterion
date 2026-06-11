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

	"github.com/google/uuid"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/SocialGouv/iterion/pkg/webhooks"
	"github.com/SocialGouv/iterion/pkg/webhooks/gitlab"
)

// registerGitLabWebhookRoute wires the inbound GitLab delivery endpoint
// behind webhookAuth. The single route dispatches on X-Gitlab-Event so
// MR opens and `/revi` note commands share one provider URL — exactly
// the path the operator pastes into GitLab's "Webhook URL" field.
func (s *Server) registerGitLabWebhookRoute() {
	s.mux.Handle("POST /api/webhooks/gitlab/{id}", s.webhookAuth(webhooks.ProviderGitLab, http.HandlerFunc(s.handleGitLabWebhook)))
}

// handleGitLabWebhook is the entry point for every GitLab delivery on
// this route. Auth, rate-limit, quota, suspend-check and tenant
// stamping are already done by webhookAuth; the config is on ctx. The
// only thing this function does itself is pick the per-event-kind sub-
// handler — everything provider-specific lives in handleGitLab* below.
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

	switch r.Header.Get("X-Gitlab-Event") {
	case gitlab.EventHeaderMergeRequest:
		s.handleGitLabMergeRequestEvent(ctx, w, r, cfg, body, payloadHash, srcIP)
	case gitlab.EventHeaderNote:
		// Conversational layer: a /revi command (or, later, a reply to
		// the bot's thread). See docs/forge-conversations.md.
		s.handleGitLabNote(ctx, w, r, cfg, body, payloadHash, srcIP)
	default:
		s.recordTerminalWebhookDelivery(ctx, cfg, webhookEventMeta{}, webhooks.StatusInvalid, payloadHash, srcIP, "unsupported X-Gitlab-Event")
		httpError(w, http.StatusBadRequest, "unsupported event (merge_request or note only)")
	}
}

// handleGitLabMergeRequestEvent handles a verified MR open/reopen. The
// merge-request path covers the auto-launch ("review on open") leg;
// pushes do NOT re-trigger (see gitlab.IsReviewable) — re-review is
// on-demand via the `/revi` Note hook below.
func (s *Server) handleGitLabMergeRequestEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, cfg webhooks.Config, body []byte, payloadHash, srcIP string) {
	p, err := gitlab.ParseMergeRequest(body)
	if err != nil {
		s.recordTerminalWebhookDelivery(ctx, cfg, webhookEventMeta{Kind: "merge_request"}, webhooks.StatusInvalid, payloadHash, srcIP, err.Error())
		httpError(w, http.StatusBadRequest, "invalid merge_request payload")
		return
	}
	meta := gitlabMRMeta(p)

	// Filter: only review on open/reopen, allowed event + project.
	// A filtered delivery returns 200 (a 4xx would make GitLab disable
	// the webhook after repeated metadata-only edits).
	if !p.IsReviewable() ||
		!gitlab.MatchEvent(cfg.EventAllowlist, "merge_request") ||
		!gitlab.MatchProject(cfg.ProjectAllowlist, p.ProjectPath) {
		s.recordTerminalWebhookDelivery(ctx, cfg, meta, webhooks.StatusFiltered, payloadHash, srcIP, "")
		writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusFiltered})
		return
	}

	botID := cfg.SelectBot()
	if botID == "" {
		botID = defaultWebhookBotReviewPR
	}
	if !cfg.AllowsBot(botID) {
		s.recordTerminalWebhookDelivery(ctx, cfg, meta, webhooks.StatusInvalid, payloadHash, srcIP, "bot not permitted by webhook scope")
		httpError(w, http.StatusForbidden, "bot %q not permitted by this webhook", botID)
		return
	}

	// Idempotency: one launch per (tenant, webhook, project, MR, head sha).
	// "mr|" prefix keeps the key space disjoint from the Note hook
	// ("note|") so a /revi on the same MR can't collide with the open.
	idemKey := knowledge.ChecksumHex([]byte(fmt.Sprintf("mr|%s|%s|%d|%d|%s", cfg.TenantID, cfg.ID, p.ProjectID, p.MRIID, p.HeadSHA)))

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

	s.insertAndLaunchWebhook(ctx, w, r, cfg, meta, idemKey, botID, vars, p.CloneURL, p.SourceBranch, payloadHash, srcIP)
}

// handleGitLabNote handles an inbound GitLab note (comment / reply) — the
// conversational path (docs/forge-conversations.md). A note triggers a
// run when it is a /revi command on an OPEN MR, the author is authorized
// (allowlist OR role-gate), and it is not the bot's own note (loop-guard).
// The launch funnels through insertAndLaunchWebhook so the per-org
// admission gate, idempotent delivery flow and metrics apply exactly as
// on every other provider path. The note's id (not the head SHA) drives
// idempotency so a fresh `/revi` after a new push re-launches.
func (s *Server) handleGitLabNote(ctx context.Context, w http.ResponseWriter, r *http.Request, cfg webhooks.Config, body []byte, payloadHash, srcIP string) {
	p, err := gitlab.ParseNote(body)
	if err != nil {
		// A structurally-valid note on a non-MR noteable (issue, commit,
		// snippet — ParseNote errors but still returns the decoded note)
		// is FILTERED with 200, not 400: operators commonly enable note
		// events broadly, and repeated 4xx make GitLab auto-disable the
		// webhook.
		if p.NoteID != 0 {
			s.recordNoteDelivery(ctx, cfg, webhooks.StatusFiltered, payloadHash, srcIP, p, "not a merge-request note")
			writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusFiltered})
			return
		}
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, p, err.Error())
		httpError(w, http.StatusBadRequest, "invalid note payload")
		return
	}
	filtered := func(reason string) {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusFiltered, payloadHash, srcIP, p, reason)
		writeJSONStatus(w, http.StatusOK, map[string]string{"status": webhooks.StatusFiltered})
	}
	if !p.IsMergeRequestNote() || p.MRState != "opened" ||
		!gitlab.MatchEvent(cfg.EventAllowlist, "note") ||
		!gitlab.MatchProject(cfg.ProjectAllowlist, p.ProjectPath) {
		filtered("out of scope (not an open-MR note / event / project)")
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
		botID = defaultWebhookBotReviewPR
	}
	if !cfg.AllowsBot(botID) {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusInvalid, payloadHash, srcIP, p, "bot not permitted by webhook scope")
		httpError(w, http.StatusForbidden, "bot %q not permitted by this webhook", botID)
		return
	}
	// Gate the replier (forge-token resolution → loop-guard → allowlist
	// OR role-gate) — externalities live behind a seam so handler tests
	// don't need a live GitLab.
	gate := s.webhookNoteGate
	if gate == nil {
		gate = s.realWebhookNoteGate
	}
	ok, reason, aerr := gate(ctx, cfg, p, botID)
	if aerr != nil {
		s.recordNoteDelivery(ctx, cfg, webhooks.StatusLaunchError, payloadHash, srcIP, p, "authz check: "+aerr.Error())
		httpError(w, http.StatusBadGateway, "authorization check failed")
		return
	}
	if !ok {
		filtered(reason)
		return
	}
	if s.logger != nil {
		s.logger.Debug("webhooks: gitlab note %s!%d /%s by %s authorized (%s)", p.ProjectPath, p.MRIID, cmd, p.AuthorUsername, reason)
	}

	// Idempotency: one launch per note.
	idemKey := knowledge.ChecksumHex([]byte(fmt.Sprintf("%s|%s|%d|%s", cfg.TenantID, cfg.ID, p.ProjectID, p.SubjectID())))

	// Launch: review-pr re-reviews the current MR state. The conversation vars
	// (discussion_id/trigger_note/replier) carry the thread context for the
	// converse bot + the forge.reply capability (A4/A5); re_review marks the
	// posted summary with the 🔁 prefix (forge-pr-review skill).
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
		"re_review":         "true",
	}
	for k, v := range cfg.LaunchVars {
		vars[k] = v
	}

	s.insertAndLaunchWebhook(ctx, w, r, cfg, gitlabNoteMeta(p), idemKey, botID, vars, p.CloneURL, p.SourceBranch, payloadHash, srcIP)
}

// recordNoteDelivery inserts a terminal note-event audit row with a
// human-readable reason (richer than the generic terminal recorder —
// the conversational path has many distinct filter causes operators
// want to tell apart in the deliveries view).
func (s *Server) recordNoteDelivery(ctx context.Context, cfg webhooks.Config, status, payloadHash, srcIP string, p gitlab.ParsedNote, errMsg string) {
	if s.webhookDeliveries == nil {
		return
	}
	d := newWebhookDelivery(cfg, gitlabNoteMeta(p), status, payloadHash, srcIP)
	d.IdempotencyKey = uuid.NewString()
	d.Error = errMsg
	_ = s.webhookDeliveries.Insert(ctx, d)
}

// realWebhookNoteGate is the production replier gate: resolve the
// bot's forge token, reject the bot's own notes (loop-guard), then
// authorize the replier via allowlist OR GitLab role-gate. Returns
// ok=false with a human filter reason for the benign refusals; err
// only for an authz infrastructure failure (502 upstream).
func (s *Server) realWebhookNoteGate(ctx context.Context, cfg webhooks.Config, p gitlab.ParsedNote, botID string) (bool, string, error) {
	// Resolve the bot's forge token (honouring per-webhook secret overrides):
	// it authenticates the auth checks AND is the identity the bot posts as.
	token, terr := s.resolveForgeToken(ctx, cfg, botID)
	if terr != nil || token == "" {
		return false, "no forge token resolved (configure a forge_token binding)", nil
	}
	api := gitlab.API{HTTP: s.httpClient, BaseURL: "https://" + hostFromURL(p.MRURL), Token: token}
	// Loop-guard: never act on the bot's own notes (else its reply re-fires).
	if bot, err := api.CurrentUser(ctx); err == nil && bot.ID == p.AuthorID {
		return false, "self note (loop-guard)", nil
	}
	// Authorization: allowlist OR role-gate.
	ok, reason, aerr := gitlab.AuthorizeReplier(ctx, api, gitlab.ReplierAuth{
		AuthorID: p.AuthorID, AuthorUsername: p.AuthorUsername, ProjectID: p.ProjectID,
		Allowlist: cfg.AuthorizedRepliers, MinRole: cfg.MinReplierRole,
	})
	if aerr != nil {
		return false, "", aerr
	}
	if !ok {
		return false, "replier not authorized: " + p.AuthorUsername, nil
	}
	return true, reason, nil
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

// gitlabMRMeta flattens a Parsed merge-request into the generic
// webhookEventMeta the shared helpers consume.
func gitlabMRMeta(p gitlab.Parsed) webhookEventMeta {
	subject := ""
	if p.MRIID != 0 {
		subject = p.SubjectID()
	}
	return webhookEventMeta{
		Kind:        "merge_request",
		Action:      p.Action,
		ProjectPath: p.ProjectPath,
		SubjectID:   subject,
		SubjectSHA:  p.HeadSHA,
	}
}

// gitlabNoteMeta flattens a ParsedNote. The audit row's EventKind is
// "note" — different from "merge_request" — so a per-tenant analytics
// query can split "auto-review on open" vs "operator-triggered /revi".
func gitlabNoteMeta(p gitlab.ParsedNote) webhookEventMeta {
	return webhookEventMeta{
		Kind:         "note",
		Action:       "comment",
		ProjectPath:  p.ProjectPath,
		SubjectID:    p.SubjectID(),
		SubjectSHA:   p.HeadSHA,
		SenderHandle: p.AuthorUsername,
	}
}

// updateWebhookDelivery is the best-effort delivery row update used by
// insertAndLaunchWebhook; an audit-store error must NOT poison the
// inbound request.
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
