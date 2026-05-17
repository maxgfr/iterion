package main

import (
	"os"

	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var botsCmd = &cobra.Command{
	Use:   "bots",
	Short: "Discover and describe available bots",
	Long: `Discover .bot files and bundle directories on disk, emit a structured
catalog used by orchestrator bots (e.g. whats-next) to pick the right bot
for an issue. Output formats: json (default), markdown, skill.

The "skill" format emits a SKILL.md ready to drop into a bundle's skills/
directory; that's the canonical way to refresh examples/whats-next/skills/
iterion-bot-catalog.md after adding or renaming a bot.`,
}

var botsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List bots discovered under one or more paths",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		paths, _ := cmd.Flags().GetStringSlice("paths")
		format, _ := cmd.Flags().GetString("format")
		if len(paths) == 0 {
			paths = []string{"examples"}
		}
		return cli.BotsList(cli.BotsListOptions{Paths: paths, Format: format}, os.Stdout)
	},
}

func init() {
	botsListCmd.Flags().StringSlice("paths", nil, "Directories or .bot files to scan (default: examples)")
	botsListCmd.Flags().String("format", "json", "Output format: json|markdown|skill")
	botsCmd.AddCommand(botsListCmd)
	rootCmd.AddCommand(botsCmd)
}
