package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/SocialGouv/iterion/pkg/supervise"
)

// SuperviseOptions configures `iterion supervise` — the attach path that
// points an LLM supervisor at an already-running run (the in-.bot
// `supervisor <name>:` declaration auto-spawns the same coordinator and
// needs none of this). The supervisor watches the run's event stream
// and enqueues steering messages the run picks up at its next turn.
type SuperviseOptions struct {
	RunID    string
	Name     string
	Model    string
	System   string   // inline policy text, or @path to read from a file
	Nodes    []string // node ids to watch (empty = whole run)
	Monitors []string // pre-declared monitors as "key=val,key=val" specs
	Cooldown time.Duration
	MaxEvals int
	StoreDir string
}

// RunSupervise attaches a supervisor to a running run and blocks until
// the run terminates or the operator interrupts (SIGINT/SIGTERM).
func RunSupervise(p *Printer, opts SuperviseOptions) error {
	if opts.RunID == "" {
		return fmt.Errorf("supervise: --run-id is required")
	}
	logger := iterlog.New(iterlog.LevelInfo, os.Stderr)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	storeDir := store.ResolveStoreDir(cwd, opts.StoreDir)

	svc, err := runview.NewService(storeDir)
	if err != nil {
		return fmt.Errorf("supervise: open store: %w", err)
	}

	system, err := resolveSystemPolicy(opts.System)
	if err != nil {
		return err
	}
	monitors, err := parseMonitorSpecs(opts.Monitors)
	if err != nil {
		return err
	}

	name := opts.Name
	if name == "" {
		name = "supervisor"
	}
	spec := supervise.Spec{
		Name:     name,
		Model:    opts.Model,
		System:   system,
		Watches:  opts.Nodes,
		Monitors: monitors,
		Cooldown: opts.Cooldown,
		MaxEvals: opts.MaxEvals,
	}

	coord := supervise.New(svc, svc, opts.RunID, spec, nil, logger)
	if coord == nil {
		return fmt.Errorf("supervise: could not start coordinator (missing service or run id)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	coord.Start(ctx)

	scope := "whole run"
	if len(opts.Nodes) > 0 {
		scope = "nodes " + strings.Join(opts.Nodes, ", ")
	}
	p.Line("Supervising run %s (%s). Press Ctrl-C to detach.", opts.RunID, scope)

	select {
	case <-ctx.Done():
	case <-coord.Done():
	}
	coord.Close()
	p.Line("Supervisor detached from run %s.", opts.RunID)
	return nil
}

// resolveSystemPolicy reads the supervision policy: an @path prefix
// loads the text from a file; otherwise the value is the inline policy.
func resolveSystemPolicy(s string) (string, error) {
	if strings.HasPrefix(s, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(s, "@"))
		if err != nil {
			return "", fmt.Errorf("supervise: read policy file: %w", err)
		}
		return string(data), nil
	}
	return s, nil
}

// parseMonitorSpecs parses repeatable --monitor flags of the form
// "event_type=tool_error,tool_name=Bash" into supervise.Monitor values.
func parseMonitorSpecs(specs []string) ([]supervise.Monitor, error) {
	var out []supervise.Monitor
	for _, spec := range specs {
		var m supervise.Monitor
		for _, kv := range strings.Split(spec, ",") {
			kv = strings.TrimSpace(kv)
			if kv == "" {
				continue
			}
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return nil, fmt.Errorf("supervise: malformed --monitor %q (want key=val)", spec)
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			switch k {
			case "event_type":
				m.EventType = v
			case "node_id":
				m.NodeID = v
			case "tool_name":
				m.ToolName = v
			case "text_contains":
				m.TextContains = v
			case "cost_gt":
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return nil, fmt.Errorf("supervise: --monitor cost_gt %q: %w", v, err)
				}
				m.CostGt = f
			default:
				return nil, fmt.Errorf("supervise: --monitor unknown key %q", k)
			}
		}
		out = append(out, m)
	}
	return out, nil
}
