package rootcmd

import "github.com/spf13/cobra"

// submitCmd is a grouping command for write-style operations
var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit reports to ICANN",
	Args:  cobra.NoArgs,
	RunE:  requireSubcommand,
}

func init() { RootCmd.AddCommand(submitCmd) }
