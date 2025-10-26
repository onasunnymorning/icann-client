package rootcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	profileFlag         string
	credentialsFileFlag string
)

// RootCmd is the base command.
var RootCmd = &cobra.Command{
	Use:          "icann",
	Short:        "ICANN client CLI",
	SilenceUsage: true, // don't print usage on runtime errors (e.g., HTTP 404)
	// We keep default error printing and also print in Execute; alternatively set SilenceErrors: true
}

// Execute runs the root command.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&profileFlag, "profile", "p", "", "credentials profile (default: env ICANN_PROFILE or 'default')")
	RootCmd.PersistentFlags().StringVarP(&credentialsFileFlag, "credentials-file", "c", "", "path to credentials file (default: env ICANN_SHARED_CREDENTIALS_FILE or ~/.icann/credentials)")
}
