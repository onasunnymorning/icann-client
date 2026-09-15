package rootcmd

import "github.com/spf13/cobra"

// tldCmd groups TLD-related read operations
var tldCmd = &cobra.Command{
	Use:   "tld",
	Short: "TLD operations",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
}

func init() { getCmd.AddCommand(tldCmd) }
