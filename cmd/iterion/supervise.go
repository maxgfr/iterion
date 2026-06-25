package main

import (
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var superviseOpts cli.SuperviseOptions

var superviseCmd = &cobra.Command{
	Use:   "supervise --run-id <id> [flags]",
	Short: "Attach an LLM supervisor to a running run",
	Long: `Attach an LLM supervisor agent to an already-running run. The
supervisor watches the run's live event stream and enqueues steering
messages the supervised agent picks up at its NEXT turn — like a human
operator watching a Claude Code session and typing a correction.

Scope it to specific agent nodes with --node (repeatable); the
supervisor is only armed while one of those nodes is the active node and
its messages are node-scoped so a late message can't leak into the next
node. Omit --node to watch the whole run.

The supervisor is event-driven: give it a --system policy describing what
to watch for, and optionally pre-declare --monitor patterns (it can also
register more at runtime). It only consults the LLM on turn boundaries
(rate-limited by --cooldown) and on monitor matches.

A run whose .bot declares a 'supervisor' block auto-spawns the same
coordinator and needs none of this.

Examples:
  iterion supervise --run-id run-123 \
    --node implement \
    --system @policies/watchdog.md \
    --monitor event_type=tool_error,tool_name=Bash

  iterion supervise --run-id run-123 --model anthropic/claude-opus-4-8 \
    --system "Stop the agent if it edits files outside src/."
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.RunSupervise(newPrinter(), superviseOpts)
	},
}

func init() {
	f := superviseCmd.Flags()
	f.StringVar(&superviseOpts.RunID, "run-id", "", "ID of the run to supervise (required)")
	f.StringVar(&superviseOpts.Name, "name", "", "Supervisor name (shown in injected messages and logs)")
	f.StringVar(&superviseOpts.Model, "model", "", "Supervisor model spec, e.g. anthropic/claude-opus-4-8 (default: auto-detect / ITERION_DEFAULT_SUPERVISOR_MODEL)")
	f.StringVar(&superviseOpts.System, "system", "", "Supervision policy text, or @path to read it from a file")
	f.StringSliceVar(&superviseOpts.Nodes, "node", nil, "Agent node id(s) to watch (repeatable; empty = whole run)")
	f.StringArrayVar(&superviseOpts.Monitors, "monitor", nil, "Pre-declared monitor as key=val,key=val (repeatable). Keys: event_type,node_id,tool_name,text_contains,cost_gt")
	f.DurationVar(&superviseOpts.Cooldown, "cooldown", 0, "Minimum time between LLM evaluations on turn boundaries (default 30s)")
	f.IntVar(&superviseOpts.MaxEvals, "max-evals", 0, "Hard cap on LLM evaluations for the run (default 20)")
	f.StringVar(&superviseOpts.StoreDir, "store-dir", "", "Override the iterion store directory")
	rootCmd.AddCommand(superviseCmd)
}
