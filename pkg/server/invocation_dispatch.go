package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/bundle"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/webhooks"
)

// commandDiscovery is the live CommandDiscovery fallback for
// webhooks.ResolveCommandRoute: it scans the bot registry for an ENABLED bot
// whose manifest invocations claim the slash-command. Used only for a
// wildcard webhook with no provisioned CommandMap (a hand-created webhook);
// orchestrator-provisioned webhooks carry an authoritative CommandMap and
// never reach this.
type commandDiscovery struct{ s *Server }

func (d commandDiscovery) LookupCommand(cmd string) (webhooks.CommandRoute, bool) {
	entries, err := botregistry.List(botregistry.ListOptions{Paths: d.s.effectivePaths()})
	if err != nil {
		return webhooks.CommandRoute{}, false
	}
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		for _, inv := range e.Invocations {
			if inv.Kind != bundle.InvocationKindCommand || inv.Command == nil {
				continue
			}
			for _, name := range append([]string{inv.Command.Name}, inv.Command.Aliases...) {
				if strings.EqualFold(strings.TrimSpace(name), cmd) {
					return commandRouteFromInvocation(e.Name, inv), true
				}
			}
		}
	}
	return webhooks.CommandRoute{}, false
}

// commandRouteFromInvocation flattens a bundle command invocation into a
// webhooks.CommandRoute. Mirrors forge.Orchestrator.buildCommandMap so the
// live-discovery fallback and the provisioned CommandMap agree.
func commandRouteFromInvocation(botID string, inv bundle.Invocation) webhooks.CommandRoute {
	return webhooks.CommandRoute{
		BotID:          botID,
		Mode:           string(inv.EffectiveMode()),
		ArgsVar:        inv.ArgsVar,
		ContextVars:    inv.ContextVars,
		Scope:          inv.Command.Scope,
		MinReplierRole: inv.Command.MinReplierRole,
		Disambiguator:  inv.Command.Disambiguator,
	}
}

// cmdDiscovery returns the live command-discovery fallback bound to this
// server (nil-safe — commandDiscovery handles registry errors internally).
func (s *Server) cmdDiscovery() webhooks.CommandDiscovery { return commandDiscovery{s: s} }

// dispatchInvocation is the shared sink a comment handler calls once it has a
// resolved command route + composed vars. It launches by execution mode:
//
//	direct → launch the run immediately (the Revi path).
//	board  → when a cloud board is wired (CloudBoardFor), materialise a
//	         tracked kanban card on the tenant's board (idempotent per comment)
//	         AND launch the run, so the operator gets a visible card linking
//	         the command to its work. Auto-dispatch of the card by a cloud
//	         dispatcher (retry/stall/human-gates) is the remaining enhancement;
//	         until then the card is a tracking record + the run executes via
//	         the normal queue. Without a cloud board (self-hosted/local) it
//	         simply launches.
//
// Keeping the switch here means a comment handler is mode-agnostic.
func (s *Server) dispatchInvocation(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	cfg webhooks.Config, meta webhookEventMeta, idemKey string,
	route webhooks.CommandRoute, vars map[string]string,
	repoURL, repoRef, payloadHash, srcIP string,
) {
	if route.Mode == string(bundle.ExecutionBoard) && s.cfg.CloudBoardFor != nil {
		s.ensureBoardCard(ctx, cfg, route, vars, meta)
	}
	s.insertAndLaunchWebhook(ctx, w, r, cfg, meta, idemKey, route.BotID, vars, repoURL, repoRef, payloadHash, srcIP)
}

// ensureBoardCard materialises a tracking kanban card for a board-mode
// command on the tenant's cloud board, idempotently: a card carrying the
// per-comment label is created at most once, so a webhook retry doesn't
// duplicate it. Best-effort — a board error never fails the command (the run
// still launches). The card is assigned to the bot (Assignee + Bot) and
// carries the command args as bot_args.
func (s *Server) ensureBoardCard(ctx context.Context, cfg webhooks.Config, route webhooks.CommandRoute, vars map[string]string, meta webhookEventMeta) {
	store := s.cfg.CloudBoardFor(cfg.TenantID)
	if store == nil {
		return
	}
	label := "cmd:" + meta.SubjectID
	if existing, err := store.List(native.ListFilter{Labels: []string{label}}); err == nil && len(existing) > 0 {
		return // already materialised for this comment
	}
	title := route.BotID
	if sn := strings.TrimSpace(vars["scope_notes"]); sn != "" {
		title = route.BotID + " — " + firstLine(sn)
	}
	botArgs := map[string]string{}
	if route.ArgsVar != "" {
		if v, ok := vars[route.ArgsVar]; ok && v != "" {
			botArgs[route.ArgsVar] = v
		}
	}
	body := fmt.Sprintf("Triggered by a /%s-style command on %s/%s.\n\n%s",
		route.BotID, meta.ProjectPath, meta.SubjectID, strings.TrimSpace(vars["scope_notes"]))
	if _, err := store.Create(native.Issue{
		Title:    truncate(title, 120),
		Body:     body,
		Assignee: route.BotID,
		Bot:      route.BotID,
		Labels:   []string{label, "source:command", "provider:" + string(cfg.Provider)},
		BotArgs:  botArgs,
	}); err != nil && s.logger != nil {
		s.logger.Warn("webhooks: board card create failed (tenant=%s bot=%s): %v", cfg.TenantID, route.BotID, err)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
