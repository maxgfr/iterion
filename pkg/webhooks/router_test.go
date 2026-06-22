package webhooks

import "testing"

type fakeDiscovery struct {
	routes map[string]CommandRoute
}

func (f fakeDiscovery) LookupCommand(cmd string) (CommandRoute, bool) {
	r, ok := f.routes[cmd]
	return r, ok
}

func TestConfigResolveCommand(t *testing.T) {
	cfg := &Config{
		CommandMap: map[string][]CommandRoute{
			"featurly": {{BotID: "feature-dev", Mode: "board", ArgsVar: "feature_prompt"}},
			// review-pr vs revi-converse share /revi, disambiguated by args.
			"revi": {
				{BotID: "review-pr", Disambiguator: "when_args_empty"},
				{BotID: "revi-converse", Disambiguator: "when_args_present", ArgsVar: "converse_question"},
			},
		},
	}

	if r, ok := cfg.ResolveCommand("featurly", ""); !ok || r.BotID != "feature-dev" || r.Mode != "board" {
		t.Errorf("featurly route: ok=%v %+v", ok, r)
	}
	// Case-insensitive command key.
	if r, ok := cfg.ResolveCommand("FEATURLY", "do X"); !ok || r.BotID != "feature-dev" {
		t.Errorf("uppercase featurly: ok=%v %+v", ok, r)
	}

	// Bare /revi → review-pr (when_args_empty).
	if r, ok := cfg.ResolveCommand("revi", ""); !ok || r.BotID != "review-pr" {
		t.Errorf("bare /revi: ok=%v %+v", ok, r)
	}
	// /revi <question> → revi-converse (when_args_present).
	if r, ok := cfg.ResolveCommand("revi", "why did you flag this?"); !ok || r.BotID != "revi-converse" {
		t.Errorf("/revi with args: ok=%v %+v", ok, r)
	}

	if _, ok := cfg.ResolveCommand("nope", ""); ok {
		t.Error("unknown command should not resolve")
	}
	if _, ok := (&Config{}).ResolveCommand("featurly", ""); ok {
		t.Error("empty CommandMap should not resolve")
	}
}

func TestResolveCommandRoute_DiscoveryFallback(t *testing.T) {
	disc := fakeDiscovery{routes: map[string]CommandRoute{"seki": {BotID: "sec-audit-source", Mode: "board"}}}

	// Wildcard webhook with no CommandMap → discovery fallback resolves.
	wild := Config{WildcardBots: true}
	if r, ok := ResolveCommandRoute(wild, "seki", "", disc); !ok || r.BotID != "sec-audit-source" {
		t.Errorf("wildcard discovery: ok=%v %+v", ok, r)
	}

	// Non-wildcard webhook with no CommandMap → NO fallback (authoritative).
	scoped := Config{BotIDs: []string{"review-pr"}}
	if _, ok := ResolveCommandRoute(scoped, "seki", "", disc); ok {
		t.Error("non-wildcard webhook must not use discovery fallback")
	}

	// CommandMap hit wins over discovery.
	mapped := Config{
		WildcardBots: true,
		CommandMap:   map[string][]CommandRoute{"seki": {{BotID: "pinned-bot"}}},
	}
	if r, ok := ResolveCommandRoute(mapped, "seki", "", disc); !ok || r.BotID != "pinned-bot" {
		t.Errorf("CommandMap should win over discovery: ok=%v %+v", ok, r)
	}

	// nil discovery → no fallback.
	if _, ok := ResolveCommandRoute(wild, "seki", "", nil); ok {
		t.Error("nil discovery should yield no route")
	}
}

func TestCommandRouteAllowsScope(t *testing.T) {
	if !(CommandRoute{}).AllowsScope("pr") {
		t.Error("empty scope should default to pr")
	}
	if (CommandRoute{}).AllowsScope("issue") {
		t.Error("empty scope should NOT match issue")
	}
	if !(CommandRoute{Scope: "any"}).AllowsScope("issue") {
		t.Error("any should match issue")
	}
	if !(CommandRoute{Scope: "issue"}).AllowsScope("issue") {
		t.Error("issue should match issue")
	}
}
