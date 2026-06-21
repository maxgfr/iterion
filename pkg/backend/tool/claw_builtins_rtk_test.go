package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SocialGouv/iterion/pkg/backend/rtk"
)

// fakeRtk drops a POSIX-sh stand-in for the rtk binary that echoes a fixed
// rewrite and exits 0, so the claw bash wrapper can be exercised without a
// real rtk install. Returns the path; sets ITERION_RTK_BIN to it.
func fakeRtk(t *testing.T, out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake rtk stand-in is a POSIX sh script")
	}
	p := filepath.Join(t.TempDir(), "rtk")
	script := "#!/bin/sh\nprintf '%s' \"" + out + "\"\nexit 0\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rtk.BinEnv, p)
}

func TestRtkRewriteBashInput(t *testing.T) {
	fakeRtk(t, "rtk git status")

	// Mode off (no ctx mode) → command untouched, same map returned.
	in := map[string]any{"command": "git status"}
	got := rtkRewriteBashInput(context.Background(), in)
	if got["command"] != "git status" {
		t.Fatalf("off: command = %v; want unchanged", got["command"])
	}

	// Mode on → command rewritten to the rtk form; original map untouched.
	orig := map[string]any{"command": "git status", "timeout": 30}
	ctx := rtk.WithMode(context.Background(), rtk.On)
	got = rtkRewriteBashInput(ctx, orig)
	if got["command"] != "rtk git status" {
		t.Fatalf("on: command = %v; want %q", got["command"], "rtk git status")
	}
	if got["timeout"] != 30 {
		t.Fatalf("on: other keys must be preserved, got %v", got["timeout"])
	}
	if orig["command"] != "git status" {
		t.Fatalf("on: caller's map was mutated (%v); want a copy", orig["command"])
	}

	// Missing/empty command → untouched.
	empty := map[string]any{}
	if got := rtkRewriteBashInput(ctx, empty); len(got) != 0 {
		t.Fatalf("empty: got %v; want empty passthrough", got)
	}
}
