package main

import (
	"fmt"
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
directory; that's the canonical way to refresh bots/whats-next/skills/
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
			paths = []string{"bots", "examples"}
		}
		return cli.BotsList(cli.BotsListOptions{Paths: paths, Format: format}, os.Stdout)
	},
}

var botsRegenCatalogCmd = &cobra.Command{
	Use:   "regen-catalog",
	Short: "Regenerate the whats-next bot catalog from bot manifests",
	Long: `Rebuild the generated region of bots/whats-next/skills/
iterion-bot-catalog.md (persona table + per-bot cards) from every bot's
manifest.yaml under the workspace, applying the .iterion/bot-overrides.yaml
catalog overlay. The runtime does this automatically at whats-next start and
the studio on every bot-metadata save; use this to refresh the committed copy
by hand after editing a manifest outside the studio.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		workdir, _ := cmd.Flags().GetString("workdir")
		if workdir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			workdir = wd
		}
		path, err := cli.BotsRegenCatalog(workdir)
		if err != nil {
			return err
		}
		if path == "" {
			fmt.Fprintf(os.Stderr, "no whats-next catalog template under %s — nothing to regenerate\n", workdir)
			return nil
		}
		fmt.Fprintln(os.Stdout, "regenerated", path)
		return nil
	},
}

func init() {
	botsListCmd.Flags().StringSlice("paths", nil, "Directories or .bot files to scan (default: bots, examples)")
	botsListCmd.Flags().String("format", "json", "Output format: json|markdown|skill")
	botsRegenCatalogCmd.Flags().String("workdir", "", "Workspace root to scan (default: current directory)")
	botsCmd.AddCommand(botsListCmd)
	botsCmd.AddCommand(botsRegenCatalogCmd)
	rootCmd.AddCommand(botsCmd)
}
