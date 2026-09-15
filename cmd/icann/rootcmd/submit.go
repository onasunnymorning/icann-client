package rootcmd

import "github.com/spf13/cobra"

// submitCmd is a grouping command for write-style operations
var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit resources",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
}

func init() { RootCmd.AddCommand(submitCmd) }
