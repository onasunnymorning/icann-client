package rootcmd

import "github.com/spf13/cobra"

// mosapiCmd groups MOSAPI-related commands
var mosapiCmd = &cobra.Command{
	Use:   "mosapi",
	Short: "MOSAPI operations",
}

func init() { RootCmd.AddCommand(mosapiCmd) }

func init() {
	// Make common MOSAPI flags persistent so they apply to all subcommands under `mosapi`.
	mosapiCmd.PersistentFlags().StringVar(&flagTLD, "tld", "", "TLD (e.g., example) [required unless in credentials]")
	mosapiCmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Environment: prod or ote")
	mosapiCmd.PersistentFlags().StringVar(&flagAuth, "auth", "", "Auth type: basic or tlsa")
	mosapiCmd.PersistentFlags().StringVar(&flagUser, "username", "", "Username for basic auth")
	mosapiCmd.PersistentFlags().StringVar(&flagPass, "password", "", "Password for basic auth")
	mosapiCmd.PersistentFlags().StringVar(&flagCertPEM, "cert-pem", "", "PEM-encoded client certificate for TLSA (string)")
	mosapiCmd.PersistentFlags().StringVar(&flagKeyPEM, "key-pem", "", "PEM-encoded client key for TLSA (string)")
	mosapiCmd.PersistentFlags().StringVar(&flagVersion, "version", "", "API version (default v2)")
	mosapiCmd.PersistentFlags().StringVar(&flagEntity, "entity", "", "Entity (default ry)")
}
