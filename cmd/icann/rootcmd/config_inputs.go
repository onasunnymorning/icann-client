package rootcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/cmd/icann/internal/cred"
)

// Flags shared by every command. They are registered as persistent flags on
// RootCmd, so each command reads them from here rather than declaring its own.
var (
	flagTLD           string
	flagEnv           string
	flagAuth          string
	flagUser          string
	flagPass          string
	flagCertPEM       string
	flagKeyPEM        string
	flagKeyPassphrase string
	flagVersion       string
	flagEntity        string
)

// buildConfigFromInputs resolves the configuration a command runs with, in
// precedence order: flags, then the credentials file, then defaults. It is the
// single place that resolution happens, so no two commands can disagree about
// which profile or TLD an invocation meant.
func buildConfigFromInputs() (base.Config, error) {
	// Choose profile: explicit --profile, otherwise default to --tld if provided.
	chosenProfile := profileFlag
	if chosenProfile == "" && flagTLD != "" {
		chosenProfile = flagTLD
	}
	rec, loadErr := cred.Load(chosenProfile, credentialsFileFlag)
	if loadErr != nil && flagAuth == "" && flagUser == "" && flagPass == "" && flagCertPEM == "" && flagKeyPEM == "" {
		return base.Config{}, loadErr
	}
	if rec == nil {
		rec = map[string]string{}
	}

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
		cfg.CertificatePEM = expandEscapes(firstNonEmpty(flagCertPEM, rec["certificate_pem"], rec["certificate"]))
		cfg.KeyPEM = expandEscapes(firstNonEmpty(flagKeyPEM, rec["key_pem"], rec["key"]))
		cfg.KeyPassphrase = firstNonEmpty(flagKeyPassphrase, rec["key_passphrase"])
	}

	if cfg.TLD == "" {
		return base.Config{}, fmt.Errorf("tld is required (provide --tld, credentials tld, or use a profile named after the TLD)")
	}
	if err := cfg.Validate(); err != nil {
		return base.Config{}, err
	}
	return cfg, nil
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

// printJSON writes v to stdout as indented JSON. Every command prints its
// result this way, so the output shape is defined in one place.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
