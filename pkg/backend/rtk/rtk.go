// Package rtk integrates rtk (https://github.com/rtk-ai/rtk, "Rust Token
// Killer") as an opt-in command-output compressor for iterion's three shell
// execution surfaces: the claude_code Bash tool, the claw bash builtin, and
// tool nodes.
//
// rtk is a single static binary that rewrites a dev command (git, cargo, npm,
// pytest, go test, grep, …) into a token-compressed equivalent — e.g.
// "git status" → "rtk git status" — saving 60–90% of the output tokens an LLM
// would otherwise consume. Its single integration primitive is the
// `rtk rewrite <cmd>` subcommand, whose documented contract is:
//
//	exit 0 + stdout : rewrite allowed                    → use stdout
//	exit 3 + stdout : rewrite, "ask" permission verdict  → use stdout
//	exit 1          : no rtk equivalent                  → run original
//	exit 2          : "deny" permission verdict          → run original
//
// Under default rtk config a rewritable command yields exit 3 (Ask), not 0
// (rtk security issue #1155: the Default verdict maps to Ask so Claude Code's
// permission layer is not bypassed). iterion uses rtk strictly as a compressor
// — never as a permission gate — and runs delegated agents under
// bypassPermissions with its own tool-policy + secret-guard, so it treats
// exit 0 and 3 identically: take the rewrite whenever stdout is non-empty and
// differs from the input command. Any failure (binary absent, timeout, deny,
// unknown command) falls back to the original command and never errors a node.
package rtk

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Environment variables understood by this package.
const (
	// ModeEnv sets the process-wide default rtk mode (on|ultra|off) used when
	// neither a node, the workflow, nor a run override sets one.
	ModeEnv = "ITERION_RTK"
	// BinEnv pins the rtk binary path, taking precedence over PATH lookup.
	BinEnv = "ITERION_RTK_BIN"
	// telemetryEnv is rtk's own opt-out switch; iterion sets it by default to
	// match its no-telemetry posture.
	telemetryEnv = "RTK_TELEMETRY_DISABLED"
)

// rewriteTimeout bounds a single `rtk rewrite` call. rtk targets <10ms
// startup; this is a generous ceiling so a wedged binary can never stall a
// node — it simply falls back to the original command.
const rewriteTimeout = 5 * time.Second

// Mode is the resolved rtk activation level for a node or run.
type Mode int

const (
	// Off disables rtk (the default).
	Off Mode = iota
	// On rewrites commands to their `rtk <cmd>` equivalent.
	On
	// Ultra rewrites to `rtk --ultra-compact <cmd>` for rtk's densest output.
	Ultra
)

func (m Mode) String() string {
	switch m {
	case On:
		return "on"
	case Ultra:
		return "ultra"
	default:
		return "off"
	}
}

// Enabled reports whether the mode performs any rewriting.
func (m Mode) Enabled() bool { return m == On || m == Ultra }

// ParseMode maps a DSL/env/CLI string to a Mode (case-insensitive). Unknown or
// empty values resolve to Off. The canonical DSL values are on|off|ultra; a few
// lenient synonyms are accepted for env/CLI ergonomics.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "auto", "true", "1", "yes", "enabled":
		return On
	case "ultra", "u", "ultra-compact", "ultracompact":
		return Ultra
	default:
		return Off
	}
}

// IsValidValue reports whether s is an accepted `rtk:` DSL value. Empty is
// valid (field unset). Used by the IR validator to flag typos.
func IsValidValue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "on", "off", "ultra":
		return true
	default:
		return false
	}
}

// Resolve picks the effective mode from iterion's precedence chain, highest
// priority first: run override (CLI --rtk / studio) > node DSL > workflow DSL >
// ITERION_RTK env default > Off. Each argument is the raw string at that level;
// "" means "unset — defer to the next level" (so a node `rtk: off` still wins
// over a workflow `rtk: on`).
func Resolve(override, node, workflow, envDefault string) Mode {
	for _, s := range []string{override, node, workflow, envDefault} {
		if strings.TrimSpace(s) != "" {
			return ParseMode(s)
		}
	}
	return Off
}

// ResolveToolNode is the rtk mode for a tool node. Tool-node output is often
// consumed deterministically (e.g. a review loop's `git diff` feeding a
// reviewer), so compression must be a deliberate per-node choice: a tool node
// compresses ONLY when its own `rtk:` field is on/ultra. A run-level override
// can force-DISABLE everything (a kill switch) but never force-ENABLE a tool
// node — a global `--rtk on` or workflow `rtk: on` must not sweep tool output
// into compression. Workflow/env defaults are intentionally ignored here.
func ResolveToolNode(override, node string) Mode {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "off", "false", "0", "no", "none", "disabled":
		return Off
	}
	return ParseMode(node)
}

// EnvDefault returns the raw ITERION_RTK value (the lowest-priority default).
func EnvDefault() string { return os.Getenv(ModeEnv) }

type ctxKey struct{}

// WithMode returns a context carrying the resolved rtk mode. The executor sets
// this before invoking a claw node's tool loop so the shared bash builtin can
// rewrite per-node (via ModeFromContext) without rebuilding the tool registry.
func WithMode(ctx context.Context, m Mode) context.Context {
	return context.WithValue(ctx, ctxKey{}, m)
}

// ModeFromContext returns the rtk mode stored by WithMode, or Off.
func ModeFromContext(ctx context.Context) Mode {
	if m, ok := ctx.Value(ctxKey{}).(Mode); ok {
		return m
	}
	return Off
}

// resolveBin is the binary resolver; a package var so tests can stub it.
var resolveBin = Locate

// Locate resolves the rtk binary path: ITERION_RTK_BIN, then PATH, then the
// conventional install locations. Returns "" when rtk is not found.
func Locate() string {
	if v := strings.TrimSpace(os.Getenv(BinEnv)); v != "" {
		if isExecutableFile(v) {
			return v
		}
	}
	if p, err := exec.LookPath("rtk"); err == nil {
		return p
	}
	for _, c := range candidateBinPaths() {
		if isExecutableFile(c) {
			return c
		}
	}
	return ""
}

// Available reports whether an rtk binary can be located.
func Available() bool { return resolveBin() != "" }

func candidateBinPaths() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".local", "bin", "rtk"))
	}
	return append(out, "/usr/local/bin/rtk", "/usr/bin/rtk")
}

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// Rewrite returns the rtk-compressed equivalent of cmd and true when rtk
// produced a usable rewrite that differs from cmd; otherwise it returns cmd
// and false. It never errors — a missing binary, timeout, non-rewrite verdict,
// or any other failure all fall back to the original command.
//
// cmd is the full shell command line, passed to rtk as a single argument
// exactly as rtk's own hook does (`rtk rewrite "$CMD"`).
func Rewrite(ctx context.Context, m Mode, cmd string) (string, bool) {
	if !m.Enabled() {
		return cmd, false
	}
	bin := resolveBin()
	if bin == "" {
		return cmd, false
	}
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return cmd, false
	}

	rctx, cancel := context.WithTimeout(ctx, rewriteTimeout)
	defer cancel()

	c := exec.CommandContext(rctx, bin, "rewrite", cmd)
	// rtk reads ~/.config/rtk/config.toml; disable telemetry by default to
	// match iterion's posture. os.Environ() is inherited first so an operator
	// who re-enables telemetry in their own env still wins.
	c.Env = append(os.Environ(), telemetryEnv+"=1")
	var stdout bytes.Buffer
	c.Stdout = &stdout
	// Stderr stays nil → /dev/null: rtk prints warnings/diagnostics there.

	err := c.Run()
	if rctx.Err() != nil {
		return cmd, false // timeout / cancellation → passthrough
	}

	// Exit 0 (allow) and 3 (ask) carry the rewrite on stdout; 1 (no
	// equivalent) and 2 (deny) do not. iterion is a compressor, not a gate,
	// so 0 and 3 are treated alike.
	if code := exitCode(err); code != 0 && code != 3 {
		return cmd, false
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" || out == trimmed {
		return cmd, false
	}
	if m == Ultra {
		out = withUltra(out)
	}
	return out, true
}

// withUltra inserts rtk's global --ultra-compact flag into a rewritten
// "rtk <cmd>" so rtk emits its densest output. No-op if out is not a plain rtk
// invocation or already carries the flag.
func withUltra(out string) string {
	const prefix = "rtk "
	if strings.HasPrefix(out, prefix) && !strings.HasPrefix(out, prefix+"--ultra-compact") {
		return prefix + "--ultra-compact " + out[len(prefix):]
	}
	return out
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
