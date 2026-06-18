package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// CurrentManifestSchema is the manifest schema version this build understands.
// Bumped only on breaking changes; minor additive fields use the reserved
// `Compat` map to avoid forcing a version bump on every new key.
const CurrentManifestSchema = 1

// Manifest is the parsed `manifest.yaml` shipped at the bundle root.
// All fields are optional except SchemaVersion, which defaults to 1 when
// omitted (treated as "explicit v1"). Future minor extensions add to
// Compat without changing SchemaVersion.
type Manifest struct {
	// Name is the bundle's technical id (falls back to the file stem
	// when empty). Distinct from the workflow's own name. Surfaced in
	// the studio's bundle picker, `iterion bots list`, and on the run
	// header next to the workflow name.
	Name string `yaml:"name"`

	// DisplayName is the bundle's friendly persona — the name an
	// operator actually uses in conversation (e.g. "Nexie" for the
	// whats-next bot, "Billy" for some future feature_dev variant).
	// Optional and free-form. When set, the studio's RunHeader gilds
	// the bot chip with a ✨ icon so dispatcher-spawned runs are
	// instantly recognisable by persona, not just by technical name.
	// Empty falls back to the Name + WorkflowName pair as before.
	DisplayName string `yaml:"display_name,omitempty"`

	// Version is a free-form bundle version string (semver or any
	// other scheme — the engine does not parse it).
	Version string `yaml:"version"`

	// Description is a one-line summary surfaced by `iterion inspect`
	// and the studio's bundle picker.
	Description string `yaml:"description"`

	// Author is a free-form attribution string.
	Author string `yaml:"author"`

	// SchemaVersion identifies the manifest format. Unknown values
	// produce a clear error pointing at the user's iterion build.
	SchemaVersion int `yaml:"schema_version"`

	// Compat is a forward-compatible bag for additive fields. Unknown
	// keys here are ignored without breaking loads from newer bundles.
	Compat map[string]interface{} `yaml:"compat,omitempty"`

	// Attachments declares default values for the workflow's
	// `attachments:` block: keys are attachment names, values are
	// paths inside the bundle's `attachments/` directory (relative).
	// Runtime uploads (Launch modal) override these.
	Attachments map[string]string `yaml:"attachments,omitempty"`

	// Triggers are free-form labels the orchestrator uses to match
	// issues to this bundle (e.g. "refactor", "feature_request").
	// Consumed by `iterion bots list` to build the bot catalog;
	// the runtime itself doesn't read them today.
	Triggers []string `yaml:"triggers,omitempty"`

	// Capabilities lists the host capabilities this bundle expects
	// to be granted (e.g. "board.create"). Documentation-only — the
	// runtime gates capabilities per node, not per bundle.
	Capabilities []string `yaml:"capabilities,omitempty"`

	// WhenToUse is the orchestrator-facing "use when" guidance for this
	// bot — the same role as the "when to use it" block in a Claude Code
	// skill. Free text, may be multi-line. Surfaced verbatim in the
	// generated iterion-bot-catalog "Use when" card that Nexie reads to
	// route a task to a bot. Optional; an empty value drops the card.
	// Edited via the studio Bot-metadata panel.
	WhenToUse string `yaml:"when_to_use,omitempty"`

	// DispatchVars maps the issue into THIS bot's input vars when the
	// dispatcher runs it (e.g. {"feature_prompt": "{{issue.title}}\n\n
	// {{issue.body}}"} for feature-dev, {"scope_notes": "…"} for a
	// reviewer). Values are dispatcher var templates ({{issue.*}}),
	// rendered per issue; per-ticket bot_args merge on top. This makes
	// the per-bot dispatch wiring DISCOVERY-DRIVEN — the stock
	// `iterion dispatch` no longer hardcodes a name→vars map; it reads
	// this from each discovered bot's manifest, so adding/renaming a bot
	// (shipped or custom) needs zero dispatcher-code edits. Optional;
	// empty = the bot receives only the global dispatch vars.
	DispatchVars map[string]string `yaml:"dispatch_vars,omitempty"`

	// Enabled toggles whether this bot is advertised in the catalog
	// exposed to orchestrator bots (Nexie). Tri-state on purpose:
	//   nil   → key absent → treated as enabled, so manifests authored
	//           before the toggle existed stay visible.
	//   true  → explicitly enabled.
	//   false → explicitly disabled: dropped from the generated catalog
	//           and not auto-dispatched, but still surfaced by the studio
	//           so an operator can flip it back on.
	// A workspace overlay (.iterion/bot-overrides.yaml) may override this
	// per-workspace without editing the manifest — see
	// botregistry.ResolveEnabled.
	Enabled *bool `yaml:"enabled,omitempty"`

	// Forge declares the forge-access requirements this bot needs to be
	// auto-provisioned onto a connected repo through the studio's
	// Integrations flow. Advisory + discovery-time metadata, like
	// DispatchVars — the runtime itself does not read this; the
	// auto-provisioning orchestrator (pkg/forge) does, to compute the
	// forge webhook events, request the right token-scope subset, and
	// create the matching webhooks.Config + bot-secret binding in one
	// transaction. Nil when the bot declares no forge ambitions (the
	// Integrations "enable on this repo" picker filters those out).
	Forge *ForgeRequirements `yaml:"forge,omitempty"`
}

// Normalized forge event vocabulary used in a manifest `forge.events`
// block. The auto-provisioner (pkg/forge) maps each entry to the
// per-provider native event when it creates the forge-side hook:
//
//	pull_request          -> gitlab "merge_requests_events",
//	                         github / forgejo "pull_request"
//	pull_request_comment  -> gitlab "note_events",
//	                         github / forgejo "issue_comment"
const (
	ForgeEventPullRequest        = "pull_request"
	ForgeEventPullRequestComment = "pull_request_comment"
)

// DefaultForgeSecretName is the workflow-secret name an integration
// binds the connection's forge token under when a manifest's
// forge.secret is empty. Matches the name review-pr / revi-converse
// declare in their .bot `secrets:` block.
const DefaultForgeSecretName = "forge_token"

// KnownForgeEvents is the closed set of normalized event names a
// manifest may declare in forge.events. decodeManifest rejects anything
// else so a typo fails fast at parse time (same bar as attachments:).
var KnownForgeEvents = map[string]bool{
	ForgeEventPullRequest:        true,
	ForgeEventPullRequestComment: true,
}

// knownForgeScopeKeys / knownForgeScopeLevels constrain a manifest
// forge.token_scopes block. The provisioner unions the keys across the
// bots co-enabled on a repo and translates them to the tightest OAuth
// scope / GitHub-App permission that satisfies the union.
var (
	knownForgeScopeKeys = map[string]bool{
		"pull_requests": true,
		"repository":    true,
		"issues":        true,
		"webhooks":      true,
	}
	knownForgeScopeLevels = map[string]bool{
		"read":  true,
		"write": true,
		"admin": true,
	}
)

// ForgeRequirements is the `forge:` block of a bundle manifest. All
// fields are optional; a bundle with no forge: block has Forge == nil.
type ForgeRequirements struct {
	// Events is the normalized event vocabulary this bot wants the
	// auto-created webhook to subscribe to (see KnownForgeEvents).
	Events []string `yaml:"events,omitempty"`

	// TokenScopes is a normalized permission map (key -> "read" |
	// "write" | "admin"); keys ∈ {pull_requests, repository, issues,
	// webhooks}. The provisioner always needs webhook-admin regardless
	// of this map — declaring it is informational. Unioned across
	// co-enabled bots to size the requested OAuth scope.
	TokenScopes map[string]string `yaml:"token_scopes,omitempty"`

	// Secret is the workflow-secret name the bundle's main.bot
	// `secrets:` block expects (e.g. "forge_token"). Empty defaults to
	// DefaultForgeSecretName. The orchestrator binds the connection's
	// managed forge token under this name; botregistry cross-references
	// it against the parsed .bot secret names.
	Secret string `yaml:"secret,omitempty"`

	// Webhook carries the launch-side knobs the orchestrator copies into
	// the auto-created webhooks.Config.
	Webhook *ForgeWebhookHints `yaml:"webhook,omitempty"`

	// Rationale is free text shown verbatim in the Integrations enable
	// dialog so the operator understands why each scope is requested.
	Rationale string `yaml:"rationale,omitempty"`
}

// ForgeWebhookHints are the webhook-launch knobs an auto-provisioned
// integration copies into webhooks.Config.
type ForgeWebhookHints struct {
	// LaunchVars are default vars the auto-created webhook stamps onto
	// every run it launches (merged with the handler defaults; operator
	// overrides still win).
	LaunchVars map[string]string `yaml:"launch_vars,omitempty"`

	// MinReplierRole mirrors webhooks.Config.MinReplierRole — the
	// minimum forge role a commenter must have to trigger the bot via a
	// note. Empty inherits the webhook default.
	MinReplierRole string `yaml:"min_replier_role,omitempty"`
}

// SecretName returns the workflow-secret name this bot binds its forge
// token under, applying DefaultForgeSecretName when unset.
func (f *ForgeRequirements) SecretName() string {
	if f == nil || strings.TrimSpace(f.Secret) == "" {
		return DefaultForgeSecretName
	}
	return f.Secret
}

// IsEnabled reports whether this bot should be advertised in the
// orchestrator-facing catalog by default. A nil Enabled (key absent
// from the manifest) is treated as enabled, so bots authored before the
// toggle existed remain visible. A workspace overlay may still override
// this — see botregistry.ResolveEnabled.
func (m *Manifest) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// LoadManifest reads and parses a manifest.yaml file. Missing file
// is not an error (returns nil, nil); only parse or schema errors fail.
func LoadManifest(path string) (*Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("bundle: read manifest %s: %w", path, err)
	}
	return decodeManifest(body, path)
}

// decodeManifest parses + validates manifest bytes (strict unmarshal,
// schema version, attachment-path safety). srcLabel names the source in
// errors. Shared by LoadManifest and WriteManifest's pre-write
// validation so a rewritten manifest is held to exactly the same bar as
// a loaded one.
func decodeManifest(body []byte, srcLabel string) (*Manifest, error) {
	var m Manifest
	if err := yaml.UnmarshalStrict(body, &m); err != nil {
		return nil, fmt.Errorf("bundle: parse manifest %s: %w", srcLabel, err)
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = CurrentManifestSchema
	}
	if m.SchemaVersion != CurrentManifestSchema {
		return nil, fmt.Errorf(
			"bundle: manifest schema_version %d not supported by this iterion build (expected %d) — upgrade iterion or downgrade the bundle",
			m.SchemaVersion, CurrentManifestSchema,
		)
	}
	// Every attachment value is later joined to the bundle's attachments/
	// directory and opened as a file by the runtime. Reject absolute or
	// "../"-escaping values at parse time so a hostile bundle can't turn
	// that join into an arbitrary host-file read. This mirrors the tar
	// extractor's guardEntry — both untrusted path sources in a .botz are
	// validated identically.
	for name, rel := range m.Attachments {
		if err := validateAttachmentRelPath(name, rel); err != nil {
			return nil, fmt.Errorf("bundle: manifest %s: %w", srcLabel, err)
		}
	}
	if err := validateForgeRequirements(m.Forge); err != nil {
		return nil, fmt.Errorf("bundle: manifest %s: %w", srcLabel, err)
	}
	return &m, nil
}

// validateForgeRequirements rejects an unknown event name or a malformed
// token-scope entry in a manifest `forge:` block at parse time, so a typo
// fails fast (same bar as attachments:). The forge.secret cross-reference
// against the bundle's main.bot `secrets:` block is a soft check surfaced
// by botregistry, not enforced here — decodeManifest does not see main.bot.
func validateForgeRequirements(f *ForgeRequirements) error {
	if f == nil {
		return nil
	}
	for _, ev := range f.Events {
		if !KnownForgeEvents[ev] {
			return fmt.Errorf("forge: unknown event %q (known: %s, %s)", ev, ForgeEventPullRequest, ForgeEventPullRequestComment)
		}
	}
	for key, level := range f.TokenScopes {
		if !knownForgeScopeKeys[key] {
			return fmt.Errorf("forge.token_scopes: unknown scope %q (known: pull_requests, repository, issues, webhooks)", key)
		}
		if !knownForgeScopeLevels[level] {
			return fmt.Errorf("forge.token_scopes[%s]: invalid level %q (want read, write, or admin)", key, level)
		}
	}
	return nil
}

// validateAttachmentRelPath rejects a manifest `attachments:` value that
// is absolute or escapes the bundle root via "..". The downstream
// consumer builds the on-disk path with a bare
// filepath.Join(AttachmentsDir, value) followed by os.Open, so an
// unvalidated value such as "../../../../etc/passwd" would read an
// arbitrary host file. Keep this in lock-step with tar.go's guardEntry.
func validateAttachmentRelPath(name, rel string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("attachment %q has an empty path", name)
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("attachment %q path must be relative, got absolute %q", name, rel)
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." {
		return fmt.Errorf("attachment %q has an empty path", name)
	}
	if strings.HasPrefix(clean, "/") {
		return fmt.Errorf("attachment %q path must be relative, got absolute %q", name, rel)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return fmt.Errorf("attachment %q path escapes the bundle (%q)", name, rel)
		}
	}
	return nil
}
