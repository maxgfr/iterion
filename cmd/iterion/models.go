package main

import (
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var modelsOpts struct {
	refresh bool
}

var modelsCmd = &cobra.Command{
	Use:   "models [provider/model-id]",
	Short: "Inspect resolved model capabilities",
	Long: `Show the ModelCapabilities iterion resolves for a model — context
window plus reasoning / tool-call / temperature support — and where each value
came from: the online aggregator (models.dev, cached under ~/.iterion) or the
curated static fallback table.

With no argument, a representative set of known models is listed. Pass an
explicit "provider/model-id" to resolve a single model. Use --refresh to
force-refetch the model-spec cache before resolving.

Examples:
  iterion models                                  # list known models
  iterion models anthropic/glm-5.2                # resolve one model
  iterion models openai/gpt-5.5 --json            # machine-readable
  iterion models --refresh                        # refresh cache, then list`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := cli.ModelsOptions{Refresh: modelsOpts.refresh}
		if len(args) == 1 {
			opts.Spec = args[0]
		}
		return cli.RunModels(cmd.Context(), opts, newPrinter())
	},
}

func init() {
	modelsCmd.Flags().BoolVar(&modelsOpts.refresh, "refresh", false,
		"Force-refetch the model-spec cache before resolving")
	rootCmd.AddCommand(modelsCmd)
}
