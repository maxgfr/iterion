package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func writePreset(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPresets_Basic(t *testing.T) {
	dir := t.TempDir()
	writePreset(t, dir, "improve-quality.md", `---
name: improve-quality
display_name: Improve Quality (SRE)
description: SRE pass
vars:
  improvement_prompt: "Focus on reliability"
  strict: true
skills: [lang-js-fallow]
---
Operate as an SRE.`)
	presets, errs := LoadPresets(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(presets) != 1 {
		t.Fatalf("want 1 preset, got %d", len(presets))
	}
	p := presets[0]
	if p.Name != "improve-quality" {
		t.Errorf("name: got %q", p.Name)
	}
	if p.DisplayName != "Improve Quality (SRE)" {
		t.Errorf("display_name: got %q", p.DisplayName)
	}
	if p.Description != "SRE pass" {
		t.Errorf("description: got %q", p.Description)
	}
	if p.Prompt != "Operate as an SRE." {
		t.Errorf("prompt: got %q", p.Prompt)
	}
	if got := p.Vars["improvement_prompt"]; got != "Focus on reliability" {
		t.Errorf("vars.improvement_prompt: got %v", got)
	}
	// A YAML-native bool stays a bool (the engine coerces against the var type).
	if got := p.Vars["strict"]; got != true {
		t.Errorf("vars.strict: got %v (want bool true)", got)
	}
	if len(p.Skills) != 1 || p.Skills[0] != "lang-js-fallow" {
		t.Errorf("skills: got %v", p.Skills)
	}
}

func TestLoadPresets_NameFallsBackToStem(t *testing.T) {
	dir := t.TempDir()
	writePreset(t, dir, "rgaa.md", "Just a body, no frontmatter.")
	presets, errs := LoadPresets(dir)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(presets) != 1 || presets[0].Name != "rgaa" {
		t.Fatalf("want name rgaa from stem, got %+v", presets)
	}
	if presets[0].Prompt != "Just a body, no frontmatter." {
		t.Errorf("prompt: %q", presets[0].Prompt)
	}
}

func TestLoadPresets_MissingDirNoError(t *testing.T) {
	if p, e := LoadPresets(""); p != nil || e != nil {
		t.Fatalf("empty dir: want nil,nil got %v %v", p, e)
	}
	if p, e := LoadPresets(filepath.Join(t.TempDir(), "nope")); p != nil || e != nil {
		t.Fatalf("missing dir: want nil,nil got %v %v", p, e)
	}
}

func TestLoadPresets_MalformedSkippedOthersLoad(t *testing.T) {
	dir := t.TempDir()
	writePreset(t, dir, "good.md", "---\nname: good\n---\nbody")
	// vars expects a map; a scalar triggers a strict-unmarshal type error.
	writePreset(t, dir, "bad.md", "---\nname: bad\nvars: not-a-map\n---\nbody")
	presets, errs := LoadPresets(dir)
	if len(errs) == 0 {
		t.Fatal("want a parse error for bad.md")
	}
	found := false
	for _, p := range presets {
		if p.Name == "good" {
			found = true
		}
	}
	if !found {
		t.Fatalf("good preset not loaded despite sibling error; got %+v", presets)
	}
}

func TestLoadPresets_UnknownKeyRejected(t *testing.T) {
	dir := t.TempDir()
	writePreset(t, dir, "x.md", "---\nname: x\nbogus_key: 1\n---\nbody")
	if _, errs := LoadPresets(dir); len(errs) == 0 {
		t.Fatal("want strict-unmarshal error for unknown frontmatter key")
	}
}

func TestLoadPresets_DuplicateNameFlagged(t *testing.T) {
	dir := t.TempDir()
	writePreset(t, dir, "a.md", "---\nname: dup\n---\nA")
	writePreset(t, dir, "b.md", "---\nname: dup\n---\nB")
	presets, errs := LoadPresets(dir)
	if len(errs) == 0 {
		t.Fatal("want a duplicate-name error")
	}
	if len(presets) != 1 {
		t.Fatalf("want 1 surviving preset, got %d", len(presets))
	}
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body := splitFrontmatter([]byte("---\nname: a\n---\nhello\nworld"))
	if string(fm) != "name: a" {
		t.Errorf("fm: %q", fm)
	}
	if string(body) != "hello\nworld" {
		t.Errorf("body: %q", body)
	}
	if fm, body := splitFrontmatter([]byte("just body")); fm != nil || string(body) != "just body" {
		t.Errorf("no-fm: fm=%q body=%q", fm, body)
	}
	// Leading blank lines before the opener are tolerated.
	if fm, _ := splitFrontmatter([]byte("\n\n---\nk: v\n---\nb")); string(fm) != "k: v" {
		t.Errorf("leading-blank fm: %q", fm)
	}
}
