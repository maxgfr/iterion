package bundle

import (
	"fmt"
	"os"

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
	var m Manifest
	if err := yaml.UnmarshalStrict(body, &m); err != nil {
		return nil, fmt.Errorf("bundle: parse manifest %s: %w", path, err)
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
	return &m, nil
}
