package rootcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	profileFlag         string
	credentialsFileFlag string
	showVersion         bool
)

// Version is set at build time via ldflags
var Version = "dev"

// RootCmd is the base command.
var RootCmd = &cobra.Command{
	Use:          "icann",
	Short:        "ICANN client CLI",
	SilenceUsage: true, // don't print usage on runtime errors (e.g., HTTP 404)
	// We keep default error printing and also print in Execute; alternatively set SilenceErrors: true
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(Version)
			return nil
		}
		return cmd.Help()
	},
}

// Execute runs the root command.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.Flags().BoolVar(&showVersion, "version", false, "Show version information")
	RootCmd.PersistentFlags().StringVarP(&profileFlag, "profile", "p", "", "credentials profile (default: env ICANN_PROFILE or 'default')")
	RootCmd.PersistentFlags().StringVarP(&credentialsFileFlag, "credentials-file", "c", "", "path to credentials file (default: env ICANN_SHARED_CREDENTIALS_FILE or ~/.icann/credentials)")

	// Global flags for target, auth, and API routing
	RootCmd.PersistentFlags().StringVar(&flagTLD, "tld", "", "TLD (e.g., example) [required unless in credentials]")
	RootCmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Environment: prod or ote")
	RootCmd.PersistentFlags().StringVar(&flagAuth, "auth", "", "Auth type: basic or tlsa")
	RootCmd.PersistentFlags().StringVar(&flagUser, "username", "", "Username for basic auth")
	RootCmd.PersistentFlags().StringVar(&flagPass, "password", "", "Password for basic auth")
	RootCmd.PersistentFlags().StringVar(&flagCertPEM, "cert-pem", "", "PEM-encoded client certificate for TLSA (string)")
	RootCmd.PersistentFlags().StringVar(&flagKeyPEM, "key-pem", "", "PEM-encoded client key for TLSA (string)")
	RootCmd.PersistentFlags().StringVar(&flagKeyPassphrase, "key-passphrase", "", "Passphrase for decrypting encrypted private key")
	RootCmd.PersistentFlags().StringVar(&flagVersion, "api-version", "", "API version (default v2)")
	RootCmd.PersistentFlags().StringVar(&flagEntity, "entity", "", "Entity (default ry)")
}
