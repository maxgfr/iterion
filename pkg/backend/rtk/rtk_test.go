package rtk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseMode(t *testing.T) {
	cases := map[string]Mode{
		"":              Off,
		"off":           Off,
		"false":         Off,
		"garbage":       Off,
		"on":            On,
		"On":            On,
		" auto ":        Off, // trimmed: "auto" is not a synonym
		"true":          On,
		"1":             On,
		"ultra":         Ultra,
		"ULTRA":         Ultra,
		"ultra-compact": Off, // trimmed: use "ultra"
	}
	for in, want := range cases {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestIsValidValue(t *testing.T) {
	for _, v := range []string{"", "on", "off", "ultra", "ON", " ultra "} {
		if !IsValidValue(v) {
			t.Errorf("IsValidValue(%q) = false; want true", v)
		}
	}
	for _, v := range []string{"auto", "true", "1", "yes", "bogus"} {
		if IsValidValue(v) {
			t.Errorf("IsValidValue(%q) = true; want false (not a canonical DSL value)", v)
		}
	}
}

func TestResolvePrecedence(t *testing.T) {
	cases := []struct {
		name                              string
		override, node, workflow, envDflt string
		want                              Mode
	}{
		{"override-wins", "ultra", "on", "off", "on", Ultra},
		{"node-off-beats-workflow-on", "", "off", "on", "ultra", Off},
		{"node-wins", "", "ultra", "on", "off", Ultra},
		{"workflow-wins", "", "", "on", "off", On},
		{"env-default", "", "", "", "ultra", Ultra},
		{"all-empty-off", "", "", "", "", Off},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.override, c.node, c.workflow, c.envDflt); got != c.want {
				t.Fatalf("Resolve(%q,%q,%q,%q) = %v; want %v", c.override, c.node, c.workflow, c.envDflt, got, c.want)
			}
		})
	}
}

func TestResolveToolNode(t *testing.T) {
	cases := []struct {
		name, override, node string
		want                 Mode
	}{
		{"node-on", "", "on", On},
		{"node-ultra", "", "ultra", Ultra},
		{"node-unset-off", "", "", Off},
		{"override-off-kills-node-on", "off", "on", Off},     // kill switch
		{"override-on-does-not-enable", "on", "", Off},       // override never force-enables a tool node
		{"override-on-keeps-node-on", "on", "on", On},        // node already opted in
		{"override-ultra-does-not-enable", "ultra", "", Off}, // only an off-ish override matters
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveToolNode(c.override, c.node); got != c.want {
				t.Fatalf("ResolveToolNode(%q,%q) = %v; want %v", c.override, c.node, got, c.want)
			}
		})
	}
}

func TestRewriteCommandField(t *testing.T) {
	fake := writeFakeRtk(t)
	old := resolveBin
	resolveBin = func() string { return fake }
	t.Cleanup(func() { resolveBin = old })
	t.Setenv("FAKE_RTK_OUT", "rtk git status")
	t.Setenv("FAKE_RTK_EXIT", "0")

	// Off → unchanged, same map, false.
	if out, changed := RewriteCommandField(context.Background(), Off, map[string]any{"command": "git status"}); changed || out["command"] != "git status" {
		t.Fatalf("off: out=%v changed=%v", out, changed)
	}
	// On → rewritten copy; other keys preserved; caller's map untouched.
	orig := map[string]any{"command": "git status", "description": "x"}
	out, changed := RewriteCommandField(context.Background(), On, orig)
	if !changed || out["command"] != "rtk git status" {
		t.Fatalf("on: out=%v changed=%v; want rewritten", out, changed)
	}
	if out["description"] != "x" {
		t.Fatalf("on: other keys dropped: %v", out)
	}
	if orig["command"] != "git status" {
		t.Fatalf("on: caller map mutated: %v", orig["command"])
	}
	// Missing command → unchanged.
	if out, changed := RewriteCommandField(context.Background(), On, map[string]any{}); changed || len(out) != 0 {
		t.Fatalf("empty: out=%v changed=%v", out, changed)
	}
}

func TestModeStringEnabled(t *testing.T) {
	if Off.Enabled() || !On.Enabled() || !Ultra.Enabled() {
		t.Fatal("Enabled() wrong")
	}
	if Off.String() != "off" || On.String() != "on" || Ultra.String() != "ultra" {
		t.Fatalf("String() wrong: %q %q %q", Off, On, Ultra)
	}
}

func TestWithUltra(t *testing.T) {
	cases := map[string]string{
		"rtk git status":                 "rtk --ultra-compact git status",
		"rtk read f --max-lines 50":      "rtk --ultra-compact read f --max-lines 50",
		"rtk --ultra-compact git status": "rtk --ultra-compact git status", // idempotent
		"git status":                     "git status",                     // not an rtk invocation
	}
	for in, want := range cases {
		if got := withUltra(in); got != want {
			t.Errorf("withUltra(%q) = %q; want %q", in, got, want)
		}
	}
}

// writeFakeRtk drops a POSIX-sh stand-in for the rtk binary that echoes
// $FAKE_RTK_OUT and exits with $FAKE_RTK_EXIT, so Rewrite can be exercised
// across the full exit-code matrix without a real rtk install.
func writeFakeRtk(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake rtk stand-in is a POSIX sh script")
	}
	p := filepath.Join(t.TempDir(), "rtk")
	script := "#!/bin/sh\nprintf '%s' \"${FAKE_RTK_OUT-}\"\nexit \"${FAKE_RTK_EXIT-0}\"\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRewrite(t *testing.T) {
	fake := writeFakeRtk(t)
	old := resolveBin
	resolveBin = func() string { return fake }
	t.Cleanup(func() { resolveBin = old })

	cases := []struct {
		name          string
		mode          Mode
		in, out, exit string
		want          string
		ok            bool
	}{
		{"allow-exit0", On, "git status", "rtk git status", "0", "rtk git status", true},
		// The common default-config case: rewritable command → Ask/exit 3, with stdout.
		{"ask-exit3", On, "git push", "rtk git push", "3", "rtk git push", true},
		{"passthrough-exit1", On, "htop", "", "1", "htop", false},
		{"deny-exit2", On, "rm -rf x", "", "2", "rm -rf x", false},
		{"already-rtk", On, "rtk git status", "rtk git status", "0", "rtk git status", false},
		{"ultra", Ultra, "git status", "rtk git status", "0", "rtk --ultra-compact git status", true},
		{"empty-out-exit0", On, "weird", "", "0", "weird", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("FAKE_RTK_OUT", c.out)
			t.Setenv("FAKE_RTK_EXIT", c.exit)
			got, ok := Rewrite(context.Background(), c.mode, c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("Rewrite(%q) = (%q, %v); want (%q, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestRewriteNoop(t *testing.T) {
	// mode Off never shells out.
	if got, ok := Rewrite(context.Background(), Off, "git status"); got != "git status" || ok {
		t.Fatalf("Off: got (%q,%v); want (%q,false)", got, ok, "git status")
	}
	// binary absent → passthrough.
	old := resolveBin
	resolveBin = func() string { return "" }
	t.Cleanup(func() { resolveBin = old })
	if got, ok := Rewrite(context.Background(), On, "git status"); got != "git status" || ok {
		t.Fatalf("absent: got (%q,%v); want (%q,false)", got, ok, "git status")
	}
	// empty/whitespace command → passthrough (no subprocess).
	if got, ok := Rewrite(context.Background(), On, "   "); got != "   " || ok {
		t.Fatalf("empty: got (%q,%v); want (%q,false)", got, ok, "   ")
	}
}

func TestLocateBinEnv(t *testing.T) {
	fake := writeFakeRtk(t)
	t.Setenv(BinEnv, fake)
	if got := Locate(); got != fake {
		t.Fatalf("Locate() = %q; want %q (ITERION_RTK_BIN)", got, fake)
	}
	if !Available() {
		t.Fatal("Available() = false with ITERION_RTK_BIN set")
	}
	// A nonexistent ITERION_RTK_BIN must be ignored, never returned.
	bogus := filepath.Join(t.TempDir(), "nope")
	t.Setenv(BinEnv, bogus)
	if got := Locate(); got == bogus {
		t.Fatalf("Locate() returned nonexistent ITERION_RTK_BIN %q", bogus)
	}
}
