package rootcmd

import (
	"encoding/json"
	"fmt"
	"io"
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
	flagRole          string
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
	entity, err := roleToEntity(firstNonEmpty(flagRole, rec["role"], rec["entity"]))
	if err != nil {
		return base.Config{}, err
	}
	cfg.Entity = entity
	cfg.AuthType = deriveAuthType(flagAuth, rec)
	switch cfg.AuthType {
	case base.AUTH_TYPE_BASIC:
		cfg.Username = firstNonEmpty(flagUser, rec["username"])
		cfg.Password = firstNonEmpty(flagPass, rec["password"])
	case base.AUTH_TYPE_CERT:
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

// roleToEntity translates the plain-language --role value (or its
// credentials-file equivalent) into the entity segment ICANN's API expects.
// "ry"/"rr" are accepted too, both because they are the values already found
// in older credentials files and because they are literally what appears in
// MOSAPI/RRI's own URLs, so a registry operator reading ICANN's own docs will
// recognize them.
func roleToEntity(role string) (string, error) {
	switch strings.ToLower(role) {
	case "", "registry", base.EntityRegistry:
		return base.EntityRegistry, nil
	case "registrar", base.EntityRegistrar:
		return base.EntityRegistrar, nil
	default:
		return "", fmt.Errorf("invalid --role %q: only \"registry\" or \"registrar\" are supported", role)
	}
}

// deriveAuthType chooses the authentication type based on precedence:
// 1) explicit flag (--auth)
// 2) credentials file key (auth_type)
// 3) presence of PEM fields implies a client certificate
// 4) default to BASIC
func deriveAuthType(explicit string, rec map[string]string) string {
	if explicit != "" {
		return strings.ToLower(explicit)
	}
	if v := strings.ToLower(rec["auth_type"]); v != "" {
		return v
	}
	if rec["certificate_pem"] != "" || rec["key_pem"] != "" {
		return base.AUTH_TYPE_CERT
	}
	return base.AUTH_TYPE_BASIC
}

// printJSON writes v as indented JSON. Every command prints its result this
// way, so the output shape is defined in one place. It takes the writer rather
// than reaching for os.Stdout so that a test can capture what a command
// printed via cmd.SetOut.
func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
