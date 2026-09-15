// Package rootcmd provides the root command structure and command grouping for the ICANN CLI.
package rootcmd

import "github.com/spf13/cobra"

// getCmd is a grouping command for read-style operations
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get resources",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
}

func init() { RootCmd.AddCommand(getCmd) }
