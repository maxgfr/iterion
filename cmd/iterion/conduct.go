package main

import (
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var conductOpts cli.ConductOptions

var conductCmd = &cobra.Command{
	Use:   "conduct <config.yaml>",
	Short: "Run the conductor daemon",
	Long: `Run the conductor daemon: poll a tracker, dispatch eligible issues
to a workflow, and expose a REST/WS surface for the editor.

Example:
  iterion conduct iterion.conductor.yaml --port 4892
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conductOpts.ConfigPath = args[0]
		return cli.RunConduct(newPrinter(), conductOpts)
	},
}

func init() {
	conductCmd.Flags().StringVar(&conductOpts.StoreDir, "store-dir", "", "Override the iterion store directory")
	conductCmd.Flags().IntVar(&conductOpts.Port, "port", 0, "HTTP port for the conductor REST/WS surface (overrides server.port in config)")
	conductCmd.Flags().BoolVar(&conductOpts.NoServer, "no-server", false, "Run headless — disable the HTTP surface even if server.port is set")
	rootCmd.AddCommand(conductCmd)
}
