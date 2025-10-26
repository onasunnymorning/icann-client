package rootcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/cmd/icann/internal/cred"
	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/spf13/cobra"
)

var (
	flagTLD     string
	flagEnv     string
	flagAuth    string
	flagUser    string
	flagPass    string
	flagCertPEM string
	flagKeyPEM  string
	flagVersion string
	flagEntity  string
)

// stateCmd fetches MOSAPI monitoring state
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Get MOSAPI monitoring state",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Choose profile: explicit --profile, otherwise default to --tld if provided.
		chosenProfile := profileFlag
		if chosenProfile == "" && flagTLD != "" {
			chosenProfile = flagTLD
		}

		// Load credentials from file/profile
		rec, loadErr := cred.Load(chosenProfile, credentialsFileFlag)
		if loadErr != nil && flagAuth == "" && flagUser == "" && flagPass == "" && flagCertPEM == "" && flagKeyPEM == "" {
			return loadErr
		}
		if rec == nil {
			rec = map[string]string{}
		}

		// Build config with precedence: flags > credentials file > defaults
		cfg := base.Config{}
		cfg.TLD = firstNonEmpty(flagTLD, rec["tld"], chosenProfile)
		cfg.Environment = firstNonEmpty(flagEnv, rec["environment"], base.ENV_PROD)
		cfg.Version = firstNonEmpty(flagVersion, rec["version"], base.V2)
		cfg.Entity = firstNonEmpty(flagEntity, rec["entity"], base.EntityRegistry)
		cfg.AuthType = deriveAuthType(flagAuth, rec)
		switch cfg.AuthType {
		case base.AUTH_TYPE_BASIC:
			cfg.Username = firstNonEmpty(flagUser, rec["username"])
			cfg.Password = firstNonEmpty(flagPass, rec["password"])
		case base.AUTH_TYPE_TLSA:
			// Prefer PEM values; support keys certificate_pem/key_pem; allow fallback to certificate/key if a caller still supplies them
			cfg.CertificatePEM = expandEscapes(firstNonEmpty(flagCertPEM, rec["certificate_pem"], rec["certificate"]))
			cfg.KeyPEM = expandEscapes(firstNonEmpty(flagKeyPEM, rec["key_pem"], rec["key"]))
		}

		if cfg.TLD == "" {
			return fmt.Errorf("tld is required (provide --tld, credentials tld, or use a profile named after the TLD)")
		}
		if err := cfg.Validate(); err != nil {
			return err
		}

		cli, err := mosapi.New(cfg)
		if err != nil {
			return err
		}
		// Execute request
		sr, err := cli.GetStateResponse()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sr)
	},
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// expandEscapes replaces common escape sequences (\n) with their literal forms.
func expandEscapes(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\\n", "\n")
	return s
}

// deriveAuthType chooses the authentication type based on precedence:
// 1) explicit flag (--auth)
// 2) credentials file key (auth_type)
// 3) presence of PEM fields implies TLSA
// 4) default to BASIC
func deriveAuthType(explicit string, rec map[string]string) string {
	if explicit != "" {
		return strings.ToLower(explicit)
	}
	if v := strings.ToLower(rec["auth_type"]); v != "" {
		return v
	}
	if rec["certificate_pem"] != "" || rec["key_pem"] != "" {
		return base.AUTH_TYPE_TLSA
	}
	return base.AUTH_TYPE_BASIC
}

func init() {
	getCmd.AddCommand(stateCmd)

	stateCmd.Flags().StringVar(&flagTLD, "tld", "", "TLD (e.g., example) [required unless in credentials]")
	stateCmd.Flags().StringVar(&flagEnv, "env", "", "Environment: prod or ote")
	stateCmd.Flags().StringVar(&flagAuth, "auth", "", "Auth type: basic or tlsa")
	stateCmd.Flags().StringVar(&flagUser, "username", "", "Username for basic auth")
	stateCmd.Flags().StringVar(&flagPass, "password", "", "Password for basic auth")
	stateCmd.Flags().StringVar(&flagCertPEM, "cert-pem", "", "PEM-encoded client certificate for TLSA (string)")
	stateCmd.Flags().StringVar(&flagKeyPEM, "key-pem", "", "PEM-encoded client key for TLSA (string)")
	stateCmd.Flags().StringVar(&flagVersion, "version", "", "API version (default v2)")
	stateCmd.Flags().StringVar(&flagEntity, "entity", "", "Entity (default ry)")

	// TLD can come from flag, credentials (tld key), or profile name matching the TLD.
}
