package bundle

// SyntheticInvocations derives the Invocation set a manifest WITHOUT an
// explicit `invocations:` block should be treated as having, from its legacy
// `forge:` block. Used by botregistry so a bundle that predates the typed
// invocations schema still participates in the Integrations picker (its
// forge-EVENT reachability is preserved). Returns nil when there's nothing
// to derive.
//
// It deliberately does NOT synthesise slash-commands: a command name can't be
// inferred generically from a forge.events entry, and inventing one would
// risk colliding with another bot's real command. In-tree bots declare their
// commands explicitly (see bots/*/manifest.yaml); this shim only keeps a
// forge:-only bundle visible as event-capable.
func SyntheticInvocations(m *Manifest) []Invocation {
	if m == nil || m.Forge == nil {
		return nil
	}
	var out []Invocation
	for _, ev := range m.Forge.Events {
		if !KnownForgeEvents[ev] {
			continue
		}
		inv := Invocation{
			Kind:  InvocationKindForge,
			Mode:  ExecutionDirect,
			Forge: &InvocationForge{Event: ev},
		}
		if ev == ForgeEventPullRequest {
			// Match the existing reviewable-action filter (open/reopen).
			inv.Forge.Actions = []string{"opened", "reopened"}
		}
		out = append(out, inv)
	}
	return out
}

// EffectiveInvocations returns the manifest's explicit invocations when
// present, else the synthetic set derived from the legacy forge: block. This
// is the single accessor every consumer (botregistry, the orchestrator,
// the command router) should use so the migration shim stays in one place.
func EffectiveInvocations(m *Manifest) []Invocation {
	if m == nil {
		return nil
	}
	if len(m.Invocations) > 0 {
		return m.Invocations
	}
	return SyntheticInvocations(m)
}
