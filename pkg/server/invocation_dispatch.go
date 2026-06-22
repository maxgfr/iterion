package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/bundle"
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
//	board  → in P1 this still launches directly; full board-issue
//	         materialisation + dispatcher pickup (self-hosted) and the cloud
//	         board land in P2. The bot still runs and receives its args; only
//	         the kanban tracking is deferred. A debug log records the deferral.
//
// Keeping the switch here means a comment handler is mode-agnostic and the P2
// board work is a single localised change.
func (s *Server) dispatchInvocation(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	cfg webhooks.Config, meta webhookEventMeta, idemKey string,
	route webhooks.CommandRoute, vars map[string]string,
	repoURL, repoRef, payloadHash, srcIP string,
) {
	if route.Mode == string(bundle.ExecutionBoard) && s.logger != nil {
		s.logger.Debug("webhooks: %s board-mode command → direct launch (board tracking lands in P2) bot=%s", cfg.Provider, route.BotID)
	}
	s.insertAndLaunchWebhook(ctx, w, r, cfg, meta, idemKey, route.BotID, vars, repoURL, repoRef, payloadHash, srcIP)
}
