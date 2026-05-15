package conductor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

func newTestLogger() *iterlog.Logger {
	return iterlog.New(iterlog.LevelInfo, &bytes.Buffer{})
}

func TestHookRunScript(t *testing.T) {
	dir := t.TempDir()
	h := &Hook{Script: "echo hello > out.txt"}
	if err := h.Run(context.Background(), newTestLogger(), "test", dir, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("expected out.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestHookRunNil(t *testing.T) {
	var h *Hook
	if err := h.Run(context.Background(), newTestLogger(), "noop", t.TempDir(), nil); err != nil {
		t.Fatalf("nil hook should be a no-op, got %v", err)
	}
}

func TestHookRunCommandFails(t *testing.T) {
	h := &Hook{Script: "exit 7"}
	err := h.Run(context.Background(), newTestLogger(), "fail", t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHookRunTimeout(t *testing.T) {
	h := &Hook{Script: "sleep 5", TimeoutMS: 100}
	start := time.Now()
	err := h.Run(context.Background(), newTestLogger(), "slow", t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("timeout took too long: %s", d)
	}
}

func TestHookRunUsesEnv(t *testing.T) {
	dir := t.TempDir()
	h := &Hook{Script: "echo $ITERION_TEST_VAR > out.txt"}
	if err := h.Run(context.Background(), newTestLogger(), "env", dir, []string{"ITERION_TEST_VAR=greenlight"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "out.txt"))
	if strings.TrimSpace(string(data)) != "greenlight" {
		t.Fatalf("env not propagated: %q", data)
	}
}

func TestHooksValidate(t *testing.T) {
	cases := []struct {
		name    string
		h       Hooks
		wantErr bool
	}{
		{"empty ok", Hooks{}, false},
		{"script only", Hooks{AfterCreate: &Hook{Script: "echo ok"}}, false},
		{"path only", Hooks{AfterCreate: &Hook{Path: "/bin/true"}}, false},
		{"both set", Hooks{BeforeRun: &Hook{Script: "x", Path: "/bin/true"}}, true},
		{"neither set", Hooks{AfterRun: &Hook{}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.h.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("want err=%v, got %v", tc.wantErr, err)
			}
		})
	}
}
