package ir

import "strings"

// Provider-routing diagnostics.
const (
	DiagUnknownProvider      DiagCode = "C087" // provider chain token outside the known set (warning)
	DiagProviderChainIgnored DiagCode = "C088" // multi-provider chain on a backend that ignores the hint (warning)
)

// KnownProviders is the set of credential-routing hints the runtime
// understands for the per-node `provider:` field (and its comma-separated
// fallback-chain form). Like KnownCapabilities this is a soft set: an
// unknown token is a warning (C087), not an error — a token may be
// meaningful to an out-of-tree backend, and env-ref forms (${VAR}) resolve
// only at run time. Mirrors the hint values matched by
// pkg/backend/delegate.anthropicCredEnvForCLI and the claw registry.
var KnownProviders = map[string]bool{
	"anthropic": true,
	"zai":       true,
	"openai":    true,
	"auto":      true,
}

// hintIgnoringBackends are the backends that do NOT consume the per-node
// provider hint today: claw derives its provider from the model-spec
// prefix and codex ignores the hint entirely. A multi-element provider
// chain on these is a no-op fall-through (the executor collapses it to the
// head), so C088 tells the author the chain won't do anything there.
var hintIgnoringBackends = map[string]bool{
	"claw":  true,
	"codex": true,
}

// validateProviders walks every LLM-capable node (agent, judge, llm
// router) and validates the `provider:` field's fallback-chain form:
//
//   - C087 (warning) for any literal chain token outside KnownProviders —
//     catches typos like "anthropc". Fields containing a ${VAR} env ref
//     are skipped wholesale: their literal text isn't the resolved value,
//     and a ${VAR:-a,b} default may itself carry commas.
//   - C088 (warning) when a >1-element chain is declared on a backend that
//     ignores the provider hint (claw / codex), so the author knows the
//     fall-through is inert there today.
//
// Both are warnings, never errors: the run still proceeds, and the
// runtime degrades gracefully (unknown hint → default precedence; chain
// on a hint-ignoring backend → first provider only).
func (c *compiler) validateProviders(w *Workflow) {
	check := func(kind, id, backend, provider string) {
		if provider == "" {
			return
		}
		// Env-ref forms resolve at run time; we can't validate the
		// literal text, and splitting a ${VAR:-a,b} default on commas
		// would misfire.
		if strings.Contains(provider, "${") {
			return
		}
		tokens := splitProviderChain(provider)
		for _, tok := range tokens {
			if tok == "auto" {
				continue
			}
			if !KnownProviders[tok] {
				c.warnfAt(DiagUnknownProvider, id, "",
					"%s %q: provider %q is not a known routing hint (known: anthropic, zai, openai, auto) — it will be ignored and the node falls back to default credential precedence",
					kind, id, tok)
			}
		}
		if len(tokens) > 1 && hintIgnoringBackends[backend] {
			c.warnfAt(DiagProviderChainIgnored, id, "",
				"%s %q: provider fallback chain %q has no effect on backend=%q (only claude_code consumes the provider hint today); the runtime uses only the first provider",
				kind, id, provider, backend)
		}
	}
	for _, n := range w.Nodes {
		switch nn := n.(type) {
		case *AgentNode:
			check("agent", nn.ID, nn.Backend, nn.Provider)
		case *JudgeNode:
			check("judge", nn.ID, nn.Backend, nn.Provider)
		case *RouterNode:
			if nn.RouterMode == RouterLLM {
				check("router", nn.ID, nn.Backend, nn.Provider)
			}
		}
	}
}

// splitProviderChain splits a literal provider field into its trimmed,
// non-empty tokens. Mirrors the runtime's resolveProviderChain so compile
// and runtime agree on what counts as a chain element.
func splitProviderChain(provider string) []string {
	parts := strings.Split(provider, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
