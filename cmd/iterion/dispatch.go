package main

import (
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/spf13/cobra"
)

var dispatchOpts cli.DispatchOptions

var dispatchCmd = &cobra.Command{
	Use:   "dispatch <config.yaml>",
	Short: "Run the dispatcher daemon",
	Long: `Run the dispatcher daemon: poll a tracker, dispatch eligible issues
to a workflow, and expose a REST/WS surface for the studio.

Example:
  iterion dispatch iterion.dispatcher.yaml --port 4892
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dispatchOpts.ConfigPath = args[0]
		return cli.RunDispatch(newPrinter(), dispatchOpts)
	},
}

func init() {
	dispatchCmd.Flags().StringVar(&dispatchOpts.StoreDir, "store-dir", "", "Override the iterion store directory")
	dispatchCmd.Flags().IntVar(&dispatchOpts.Port, "port", 0, "HTTP port for the dispatcher REST/WS surface (overrides server.port in config)")
	dispatchCmd.Flags().BoolVar(&dispatchOpts.NoServer, "no-server", false, "Run headless — disable the HTTP surface even if server.port is set")
	rootCmd.AddCommand(dispatchCmd)
}
