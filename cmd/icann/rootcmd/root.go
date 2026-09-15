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
	Use:   "icann",
	Short: "Report a gTLD registry's ICANN compliance data",
	Long: `icann reports a gTLD registry's compliance data to ICANN.

It covers the operational obligations a registry operator carries day to
day:

  - Registry data escrow: submit deposits and check whether ICANN has
    received and accepted them (see 'icann submit escrow', 'icann get escrow status').
  - Specification 3 monthly reports: submit and check the per-registrar
    transactions and registry-functions-activity reports ('icann submit
    monthly', 'icann get monthly status').
  - SLA monitoring and domain-abuse (METRICA/DAAR) reports for the TLD
    ('icann get tld status', 'icann get abuse ...').
  - ICANN's own view of which reporting obligations are outstanding
    ('icann get reporting status'), and which RRI specifications ICANN
    implements ('icann get conformance').

Run 'icann status' for a one-shot summary of all of the above for a TLD.

Every command needs a TLD and credentials, resolved from flags, environment
variables, or a credentials file — run 'icann config show' to see what a
command would actually use.`,
	SilenceUsage: true, // don't print usage on runtime errors (e.g., HTTP 404)
	// Errors are reported by Execute, which also controls the exit code.
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(Version)
			return nil
		}
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return cmd.Help()
	},
}

// requireSubcommand is used as RunE on every grouping command: one that holds
// only subcommands and takes no action of its own. Cobra checks
// Args: cobra.NoArgs only once a command is Runnable, and a command with no
// RunE at all is not — without this, an unknown subcommand (e.g. "icann get
// foo") silently prints help and exits 0 instead of failing.
func requireSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
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
	RootCmd.PersistentFlags().StringVar(&flagAuth, "auth", "", "Auth type: basic (username/password) or cert (TLS client certificate)")
	RootCmd.PersistentFlags().StringVar(&flagUser, "username", "", "Username for basic auth")
	RootCmd.PersistentFlags().StringVar(&flagPass, "password", "", "Password for basic auth")
	RootCmd.PersistentFlags().StringVar(&flagCertPEM, "cert-pem", "", "PEM-encoded client certificate, for --auth cert (string)")
	RootCmd.PersistentFlags().StringVar(&flagKeyPEM, "key-pem", "", "PEM-encoded client key, for --auth cert (string)")
	RootCmd.PersistentFlags().StringVar(&flagKeyPassphrase, "key-passphrase", "", "Passphrase for decrypting encrypted private key")
	RootCmd.PersistentFlags().StringVar(&flagVersion, "api-version", "", "API version (default v2)")
	RootCmd.PersistentFlags().StringVar(&flagRole, "role", "", "Which side of the API you're calling as: registry or registrar (default registry)")
}
