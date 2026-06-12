package main

import (
	"fmt"
	"os"

	"github.com/SocialGouv/iterion/pkg/botinstall"
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

var botsInstallCmd = &cobra.Command{
	Use:   "install <git-url|path>",
	Short: "Install a bot bundle from a git repository or local path",
	Long: `Import a bot bundle from a git URL (optionally URL#ref) or a local
directory into the workspace, so iterion bots list, the dispatcher, and the
studio discover it.

A source repository publishes bots by holding bundle directories (each a
main.bot + manifest.yaml) and, optionally, an iterion-bots.yaml index at its
root listing them. When the repo holds a single bundle it is installed
directly; when it holds several, pass --path <subdir|name> to choose one.

Installed bots are NEVER run automatically — inspect, then launch (run-time
sandboxing applies as usual). By default bots install under <workdir>/.botz/
(git-ignored); pass --dest bots to install into a committable location.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, _ := cmd.Flags().GetString("ref")
		path, _ := cmd.Flags().GetString("path")
		dest, _ := cmd.Flags().GetString("dest")
		name, _ := cmd.Flags().GetString("name")
		force, _ := cmd.Flags().GetBool("force")
		workdir, _ := cmd.Flags().GetString("workdir")
		p := newPrinter()
		res, err := botinstall.Install(cmd.Context(), botinstall.Options{
			Source: args[0], Ref: ref, Path: path, Dest: dest, Name: name, Force: force, Workdir: workdir,
		})
		if err != nil {
			return err
		}
		if p.Format == cli.OutputJSON {
			p.JSON(res)
			return nil
		}
		p.Header("Bot installed")
		p.KV("Name", res.Name)
		p.KV("From", res.Source)
		if res.Ref != "" {
			p.KV("Ref", res.Ref)
		}
		p.KV("Path", res.InstalledPath)
		p.KV("Skills", fmt.Sprintf("%d", res.Skills))
		p.KV("Presets", fmt.Sprintf("%d", res.Presets))
		p.Blank()
		p.Line("  Inspect it, then launch:")
		p.Line("    iterion run %s", res.InstalledPath)
		return nil
	},
}

func init() {
	botsListCmd.Flags().StringSlice("paths", nil, "Directories or .bot files to scan (default: bots, examples)")
	botsListCmd.Flags().String("format", "json", "Output format: json|markdown|skill")
	botsRegenCatalogCmd.Flags().String("workdir", "", "Workspace root to scan (default: current directory)")
	botsInstallCmd.Flags().String("ref", "", "Git ref (branch or tag) to clone")
	botsInstallCmd.Flags().String("path", "", "Subdirectory or iterion-bots.yaml bot name to install when the repo holds several")
	botsInstallCmd.Flags().String("dest", "", "Install destination root (default: <workdir>/.botz)")
	botsInstallCmd.Flags().String("name", "", "Install under this name instead of the source's")
	botsInstallCmd.Flags().Bool("force", false, "Overwrite an existing install of the same name")
	botsInstallCmd.Flags().String("workdir", "", "Workspace root for catalog refresh (default: current directory)")
	botsCmd.AddCommand(botsListCmd)
	botsCmd.AddCommand(botsRegenCatalogCmd)
	botsCmd.AddCommand(botsInstallCmd)
	rootCmd.AddCommand(botsCmd)
}
