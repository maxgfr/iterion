package botregistry

import (
	"path/filepath"
	"testing"
)

// TestLoadSchema_FilePresetsSurface verifies a bundle's presets/<name>.md
// files surface through LoadSchema (the path the studio Launch picker reads)
// with their rich metadata + typed var values.
func TestLoadSchema_FilePresetsSurface(t *testing.T) {
	dir := filepath.Join("..", "..", "bots", "whole-improve-loop")
	e := Entry{Name: "whole-improve-loop", Path: dir, IsBundleDir: true}
	ClearSchemaCache()
	_, presets, err := LoadSchema(e)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if presets == nil || len(presets.Entries) == 0 {
		t.Fatal("expected file presets to surface, got none")
	}
	byName := map[string]*Preset{}
	for _, p := range presets.Entries {
		byName[p.Name] = p
	}
	iq := byName["improve-quality"]
	if iq == nil {
		t.Fatal("improve-quality preset missing from schema")
	}
	if iq.DisplayName == "" || iq.Prompt == "" {
		t.Errorf("improve-quality: display_name=%q prompt_len=%d", iq.DisplayName, len(iq.Prompt))
	}
	if len(iq.Skills) == 0 || iq.Skills[0] != "lang-js-fallow" {
		t.Errorf("improve-quality skills: %v", iq.Skills)
	}
	var found bool
	for _, v := range iq.Values {
		if v.Key == "improvement_prompt" {
			found = true
			if v.Value == nil || v.Value.Kind != "string" || v.Value.StrVal == "" {
				t.Errorf("improvement_prompt literal not a non-empty string: %+v", v.Value)
			}
		}
	}
	if !found {
		t.Error("improve-quality missing improvement_prompt value")
	}
}
