package rootcmd

import "github.com/spf13/cobra"

// getCmd is a grouping command for read-style operations
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get resources",
}

func init() { RootCmd.AddCommand(getCmd) }
