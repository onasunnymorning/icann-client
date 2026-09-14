package rootcmd

import "github.com/spf13/cobra"

// submitCmd is a grouping command for write-style operations
var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit resources",
}

func init() { RootCmd.AddCommand(submitCmd) }
