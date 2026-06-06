package botregistry

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"
)

// overridesRelPath is the workspace-local file that overrides per-bot
// catalog visibility without editing a bot's manifest. It lives under
// the already-gitignored .iterion/ store dir, so toggling a shipped
// (git-tracked) bot off in a workspace produces no source churn. The
// studio Catalog manager writes it; operators may hand-edit it. Overlay
// state WINS over the manifest's `enabled` default.
const overridesRelPath = ".iterion/bot-overrides.yaml"

// Overrides is the parsed workspace overlay. Keys are canonicalised bot
// names (NormalizeName); each entry may pin `enabled` either way, so the
// overlay can both hide a manifest-enabled bot and re-expose a
// manifest-disabled one.
type Overrides struct {
	Bots map[string]BotOverride `yaml:"bots,omitempty"`
}

// BotOverride is the per-bot workspace override. A nil Enabled means the
// entry carries no visibility decision (the manifest default stands).
type BotOverride struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// LoadOverrides reads <workdir>/.iterion/bot-overrides.yaml. A missing
// file is not an error — it returns an empty, non-nil Overrides.
func LoadOverrides(workdir string) (*Overrides, error) {
	path := filepath.Join(workdir, overridesRelPath)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Overrides{}, nil
		}
		return nil, fmt.Errorf("botregistry: read overrides %s: %w", path, err)
	}
	var ov Overrides
	if err := yaml.Unmarshal(body, &ov); err != nil {
		return nil, fmt.Errorf("botregistry: parse overrides %s: %w", path, err)
	}
	return &ov, nil
}

// lookup returns the override for name (matched by normalized name).
func (o *Overrides) lookup(name string) (BotOverride, bool) {
	if o == nil || o.Bots == nil {
		return BotOverride{}, false
	}
	bo, ok := o.Bots[NormalizeName(name)]
	return bo, ok
}

// ResolveEnabled composes a bot's manifest `enabled` default with the
// workspace overlay: an overlay entry that pins `enabled` WINS, otherwise
// the manifest default stands. ov may be nil (= no overlay).
func ResolveEnabled(name string, manifestEnabled bool, ov *Overrides) bool {
	if bo, ok := ov.lookup(name); ok && bo.Enabled != nil {
		return *bo.Enabled
	}
	return manifestEnabled
}

// SetOverlayEnabled records a per-bot visibility override and persists it
// atomically. enabled==nil clears any override (the manifest default
// stands again); a non-nil enabled pins the bot on/off in this workspace.
func SetOverlayEnabled(workdir, name string, enabled *bool) error {
	ov, err := LoadOverrides(workdir)
	if err != nil {
		return err
	}
	if ov.Bots == nil {
		ov.Bots = map[string]BotOverride{}
	}
	key := NormalizeName(name)
	if enabled == nil {
		delete(ov.Bots, key)
	} else {
		ov.Bots[key] = BotOverride{Enabled: enabled}
	}
	return writeOverrides(workdir, ov)
}

// writeOverrides marshals ov to <workdir>/.iterion/bot-overrides.yaml via
// a sibling temp + rename.
func writeOverrides(workdir string, ov *Overrides) error {
	dir := filepath.Join(workdir, ".iterion")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("botregistry: mkdir %s: %w", dir, err)
	}
	body, err := yaml.Marshal(ov)
	if err != nil {
		return fmt.Errorf("botregistry: marshal overrides: %w", err)
	}
	path := filepath.Join(workdir, overridesRelPath)
	tmp, err := os.CreateTemp(dir, "bot-overrides.*.tmp")
	if err != nil {
		return fmt.Errorf("botregistry: tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("botregistry: write overrides: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("botregistry: close overrides: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("botregistry: rename overrides: %w", err)
	}
	return nil
}
