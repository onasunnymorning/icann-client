package rootcmd

import "github.com/spf13/cobra"

// mosapiCmd groups MOSAPI-related commands
var mosapiCmd = &cobra.Command{
	Use:   "mosapi",
	Short: "MOSAPI operations",
}

func init() { RootCmd.AddCommand(mosapiCmd) }
