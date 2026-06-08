package model

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/SocialGouv/iterion/pkg/backend/secretguard"
	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/store"
)

// BuildSecretGuard assembles the per-run secret guard (Layer 0/1/2)
// from every value iterion can already see as sensitive:
//
//   - the resolved per-run credentials carried in ctx (cloud BYOK keys),
//   - host env vars whose name looks secret (the local-run path — same
//     definition store.CaptureLaunchEnv uses), and
//   - the workflow's declared `secrets:` values (Layer 1; the agent only
//     ever sees their placeholder).
//
// Returns nil when redaction is disabled via ITERION_SECRETS_REDACT=off,
// in which case every guard method is a no-op. Otherwise returns a
// non-nil guard even with zero known values, so the heuristic detector
// pass still scrubs unknown token shapes from the run's sinks.
func BuildSecretGuard(ctx context.Context, wf *ir.Workflow, vars map[string]string) *secretguard.Guard {
	if !envFlagEnabled("ITERION_SECRETS_REDACT", true) {
		return nil
	}

	var known []secretguard.Secret

	// 1. Resolved credentials (cloud / runner-injected).
	if creds, ok := secrets.CredentialsFromContext(ctx); ok {
		for prov, val := range creds.APIKeys {
			if val == "" {
				continue
			}
			known = append(known, secretguard.Secret{
				Name:  "provider_" + string(prov),
				Value: val,
			})
		}
	}

	// 2. Sensitive host env vars (local runs, where keys come from the
	//    environment rather than an injected bundle).
	for _, raw := range os.Environ() {
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			continue
		}
		name, val := raw[:eq], raw[eq+1:]
		if val == "" || !store.IsSecretEnvName(name) {
			continue
		}
		known = append(known, secretguard.Secret{
			Name:  "env_" + name,
			Value: val,
		})
	}

	// 3. Declared workflow secrets (Layer 1). These carry an explicit
	//    placeholder the agent sees in place of the value, plus optional
	//    egress host scoping consumed by Layer 2.
	known = append(known, declaredWorkflowSecrets(wf, vars)...)

	return secretguard.New(known, secretGuardConfigFromEnv())
}

// declaredWorkflowSecrets resolves the workflow's `secrets:` block into
// guard entries. The DSL block is wired by Layer 1; until then this
// returns nil (no declared secrets), keeping Layer 0 self-contained.
func declaredWorkflowSecrets(wf *ir.Workflow, vars map[string]string) []secretguard.Secret {
	if wf == nil {
		return nil
	}
	// Layer 1 will read wf.Secrets here and resolve ${VAR}/vars values
	// into secretguard.Secret entries with explicit placeholders + host
	// scoping. No-op for Layer 0.
	return nil
}

// secretGuardConfigFromEnv resolves the guard's tunables from the
// environment, falling back to the production defaults.
func secretGuardConfigFromEnv() secretguard.Config {
	cfg := secretguard.DefaultConfig()
	if !envFlagEnabled("ITERION_SECRETS_REDACT_HEURISTIC", true) {
		cfg.Heuristic = false
	}
	if !envFlagEnabled("ITERION_SECRETS_REDACT_DECODE", true) {
		cfg.RecurseDecode = false
	}
	if v := strings.TrimSpace(os.Getenv("ITERION_SECRETS_REDACT_MIN_SCORE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.MinScore = f
		}
	}
	return cfg
}

// envFlagEnabled reads a boolean-ish env var. Recognised falsey values
// are "0", "false", "off", "no" (case-insensitive); anything else
// (including unset) returns def.
func envFlagEnabled(name string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	default:
		return def
	}
}
