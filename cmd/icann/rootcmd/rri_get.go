package rootcmd

import "github.com/spf13/cobra"

// rriGetCmd groups read-style operations for RRI
var rriGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get RRI resources",
}

func init() { rriCmd.AddCommand(rriGetCmd) }
