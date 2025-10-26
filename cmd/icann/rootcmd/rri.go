package rootcmd

import "github.com/spf13/cobra"

// rriCmd groups RRI-related commands
var rriCmd = &cobra.Command{
	Use:   "rri",
	Short: "RRI operations",
}

func init() { RootCmd.AddCommand(rriCmd) }

func init() {
	// Make common flags persistent so they apply to all subcommands under `rri`.
	rriCmd.PersistentFlags().StringVar(&flagTLD, "tld", "", "TLD (e.g., example) [required unless in credentials]")
	rriCmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Environment: prod or ote")
	rriCmd.PersistentFlags().StringVar(&flagAuth, "auth", "", "Auth type: basic or tlsa")
	rriCmd.PersistentFlags().StringVar(&flagUser, "username", "", "Username for basic auth")
	rriCmd.PersistentFlags().StringVar(&flagPass, "password", "", "Password for basic auth")
	rriCmd.PersistentFlags().StringVar(&flagCertPEM, "cert-pem", "", "PEM-encoded client certificate for TLSA (string)")
	rriCmd.PersistentFlags().StringVar(&flagKeyPEM, "key-pem", "", "PEM-encoded client key for TLSA (string)")
	rriCmd.PersistentFlags().StringVar(&flagVersion, "version", "", "API version (default v2)")
	rriCmd.PersistentFlags().StringVar(&flagEntity, "entity", "", "Entity (default ry)")
}
