package main

import (
	"fmt"
	"strings"

	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var marketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Browse, submit, and install bots from the local hosted registry",
	Long: `Operate the local bot marketplace — the same registry the studio's
Marketplace view reads (stored at <store-dir>/marketplace/marketplace.json).

  iterion marketplace list                 # browse the registry
  iterion marketplace submit <url|path>    # validate + index a bot's repo
  iterion marketplace install <slug>       # install a listed bot into .botz/

Submitting only indexes a repo's metadata (it does not install). Installing
resolves the entry's repo coordinates and copies the bundle into the
workspace's .botz/ — bots are never run automatically.`,
}

var marketplaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List bots in the local registry",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		storeDir, _ := cmd.Flags().GetString("store-dir")
		q, _ := cmd.Flags().GetString("query")
		tag, _ := cmd.Flags().GetString("tag")
		entries, err := cli.MarketplaceList(cmd.Context(), cli.MarketplaceListOptions{
			StoreDir: storeDir, Text: q, Tag: tag,
		})
		if err != nil {
			return err
		}
		p := newPrinter()
		if p.Format == cli.OutputJSON {
			p.JSON(map[string]any{"bots": entries})
			return nil
		}
		if len(entries) == 0 {
			p.Line("No bots in the marketplace yet.")
			return nil
		}
		p.Header(fmt.Sprintf("%d bot(s)", len(entries)))
		for _, e := range entries {
			label := e.DisplayName
			if label == "" {
				label = e.Name
			}
			p.Line("  %-22s %3d install(s)  %s", e.Slug, e.Installs, label)
			if e.Description != "" {
				p.Line("      %s", e.Description)
			}
			if len(e.Tags) > 0 {
				p.Line("      tags: %s", strings.Join(e.Tags, ", "))
			}
		}
		return nil
	},
}

var marketplaceSubmitCmd = &cobra.Command{
	Use:   "submit <git-url|path>",
	Short: "Validate a repository and index the bot it publishes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storeDir, _ := cmd.Flags().GetString("store-dir")
		ref, _ := cmd.Flags().GetString("ref")
		path, _ := cmd.Flags().GetString("path")
		tags, _ := cmd.Flags().GetStringSlice("tag")
		entry, err := cli.MarketplaceSubmit(cmd.Context(), cli.MarketplaceSubmitOptions{
			StoreDir: storeDir, Source: args[0], Ref: ref, Path: path, Tags: tags,
		})
		if err != nil {
			return err
		}
		p := newPrinter()
		if p.Format == cli.OutputJSON {
			p.JSON(entry)
			return nil
		}
		p.Header("Submitted to the marketplace")
		p.KV("Slug", entry.Slug)
		p.KV("Name", entry.Name)
		if entry.Version != "" {
			p.KV("Version", entry.Version)
		}
		p.KV("Repo", entry.RepoURL)
		p.Blank()
		p.Line("  Install it with:")
		p.Line("    iterion marketplace install %s", entry.Slug)
		return nil
	},
}

var marketplaceInstallCmd = &cobra.Command{
	Use:   "install <slug>",
	Short: "Install a listed bot into the workspace's .botz/",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storeDir, _ := cmd.Flags().GetString("store-dir")
		workdir, _ := cmd.Flags().GetString("workdir")
		force, _ := cmd.Flags().GetBool("force")
		res, entry, err := cli.MarketplaceInstall(cmd.Context(), cli.MarketplaceInstallOptions{
			StoreDir: storeDir, Slug: args[0], Workdir: workdir, Force: force,
		})
		if err != nil {
			return err
		}
		p := newPrinter()
		if p.Format == cli.OutputJSON {
			p.JSON(map[string]any{"install": res, "entry": entry})
			return nil
		}
		p.Header("Bot installed")
		p.KV("Name", res.Name)
		p.KV("Path", res.InstalledPath)
		p.KV("Installs", fmt.Sprintf("%d", entry.Installs))
		p.Blank()
		p.Line("  Inspect it, then launch:")
		p.Line("    iterion run %s", res.InstalledPath)
		return nil
	},
}

func init() {
	marketplaceListCmd.Flags().String("store-dir", "", "Store directory (default: .iterion)")
	marketplaceListCmd.Flags().StringP("query", "q", "", "Free-text filter (name/description/tag)")
	marketplaceListCmd.Flags().String("tag", "", "Exact tag filter")

	marketplaceSubmitCmd.Flags().String("store-dir", "", "Store directory (default: .iterion)")
	marketplaceSubmitCmd.Flags().String("ref", "", "Git ref (branch or tag) to clone")
	marketplaceSubmitCmd.Flags().String("path", "", "Subdirectory or iterion-bots.yaml bot name when the repo holds several")
	marketplaceSubmitCmd.Flags().StringSlice("tag", nil, "Marketplace tags (repeatable)")

	marketplaceInstallCmd.Flags().String("store-dir", "", "Store directory (default: .iterion)")
	marketplaceInstallCmd.Flags().String("workdir", "", "Workspace root to install into (default: current directory)")
	marketplaceInstallCmd.Flags().Bool("force", false, "Overwrite an existing install (update)")

	marketplaceCmd.AddCommand(marketplaceListCmd)
	marketplaceCmd.AddCommand(marketplaceSubmitCmd)
	marketplaceCmd.AddCommand(marketplaceInstallCmd)
	rootCmd.AddCommand(marketplaceCmd)
}
