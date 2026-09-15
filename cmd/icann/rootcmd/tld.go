package rootcmd

import "github.com/spf13/cobra"

// tldCmd groups TLD-related read operations
var tldCmd = &cobra.Command{
	Use:   "tld",
	Short: "TLD SLA monitoring",
	Args:  cobra.NoArgs,
	RunE:  requireSubcommand,
}

func init() { getCmd.AddCommand(tldCmd) }
