package webhooks

// CommandDiscovery is the live fallback resolver used when a webhook carries
// no provisioned CommandMap entry for a slash-command. It lets a hand-created
// WILDCARD webhook still route `/featurly`-style commands by asking the bot
// registry which enabled bot claims the command. Implemented in pkg/server by
// a botregistry-backed adapter; nil disables the fallback entirely.
type CommandDiscovery interface {
	// LookupCommand returns the route an enabled bot declares for cmd
	// (lowercase, no leading slash), or ok=false when none does.
	LookupCommand(cmd string) (CommandRoute, bool)
}

// ResolveCommandRoute resolves a /slash-command to a route, the shared entry
// point every provider's comment handler calls after parsing a note.
//
// Resolution order:
//  1. The per-webhook CommandMap (the provisioned, scoped index). This is
//     authoritative: for a NON-wildcard webhook an unknown command must NOT
//     silently resolve to some other bot, so we stop here.
//  2. Only when the webhook is wildcard, a live discovery fallback — and the
//     resolved bot must still pass AllowsBot (defence in depth; a wildcard
//     webhook allows any bot, but this keeps the contract explicit).
//
// ok=false means nothing matched; the caller filters the delivery (200, never
// 4xx, so the forge doesn't auto-disable the webhook).
func ResolveCommandRoute(cfg Config, cmd, args string, discovery CommandDiscovery) (CommandRoute, bool) {
	if r, ok := cfg.ResolveCommand(cmd, args); ok {
		return r, true
	}
	if cfg.WildcardBots && discovery != nil {
		if r, ok := discovery.LookupCommand(cmd); ok && cfg.AllowsBot(r.BotID) {
			return r, true
		}
	}
	return CommandRoute{}, false
}
