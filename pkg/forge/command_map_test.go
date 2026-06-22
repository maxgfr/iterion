package forge

import (
	"testing"

	"github.com/SocialGouv/iterion/pkg/bundle"
)

func invLookup(m map[string][]bundle.Invocation) BotInvocationsLookup {
	return func(botID string) ([]bundle.Invocation, error) { return m[botID], nil }
}

func cmdInv(name, mode, argsVar, disamb string, aliases ...string) bundle.Invocation {
	return bundle.Invocation{
		Kind:    bundle.InvocationKindCommand,
		Mode:    bundle.ExecutionMode(mode),
		ArgsVar: argsVar,
		Command: &bundle.InvocationCommand{Name: name, Aliases: aliases, Disambiguator: disamb},
	}
}

func TestBuildCommandMap_SingleBotAndAliases(t *testing.T) {
	o := &Orchestrator{Invocations: invLookup(map[string][]bundle.Invocation{
		"feature-dev": {cmdInv("featurly", "board", "feature_prompt", "", "feature-dev")},
	})}
	m, err := o.buildCommandMap([]string{"feature-dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"featurly", "feature-dev"} {
		routes, ok := m[key]
		if !ok || len(routes) != 1 {
			t.Fatalf("key %q: want 1 route, got %v", key, routes)
		}
		if routes[0].BotID != "feature-dev" || routes[0].Mode != "board" || routes[0].ArgsVar != "feature_prompt" {
			t.Errorf("key %q route: %+v", key, routes[0])
		}
	}
}

func TestBuildCommandMap_ArgsDisambiguation(t *testing.T) {
	o := &Orchestrator{Invocations: invLookup(map[string][]bundle.Invocation{
		"review-pr":     {cmdInv("revi", "direct", "", "when_args_empty")},
		"revi-converse": {cmdInv("revi", "direct", "converse_question", "when_args_present")},
	})}
	m, err := o.buildCommandMap([]string{"review-pr", "revi-converse"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(m["revi"]); got != 2 {
		t.Fatalf("revi: want 2 disambiguated routes, got %d", got)
	}
}

func TestBuildCommandMap_CollisionRejected(t *testing.T) {
	o := &Orchestrator{Invocations: invLookup(map[string][]bundle.Invocation{
		"bot-a": {cmdInv("dup", "direct", "", "")},
		"bot-b": {cmdInv("dup", "direct", "", "")},
	})}
	_, err := o.buildCommandMap([]string{"bot-a", "bot-b"})
	if err == nil {
		t.Fatal("expected collision error for two bots claiming /dup without disambiguation")
	}
}

func TestBuildCommandMap_NilWhenNoCommands(t *testing.T) {
	// Lookup wired but the bot declares only a forge invocation.
	o := &Orchestrator{Invocations: invLookup(map[string][]bundle.Invocation{
		"review-pr": {{Kind: bundle.InvocationKindForge, Forge: &bundle.InvocationForge{Event: bundle.ForgeEventPullRequest}}},
	})}
	m, err := o.buildCommandMap([]string{"review-pr"})
	if err != nil || m != nil {
		t.Errorf("want nil map, got %v (err=%v)", m, err)
	}

	// No lookup wired at all → nil.
	if m, _ := (&Orchestrator{}).buildCommandMap([]string{"x"}); m != nil {
		t.Errorf("nil Invocations should yield nil map, got %v", m)
	}
}
