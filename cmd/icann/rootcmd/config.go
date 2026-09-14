package rootcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/cmd/icann/internal/cred"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect resolved configuration and credentials",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show which credentials and endpoints a command would use",
	Long: `Show the configuration a command would run with, after merging flags,
the credentials file and defaults.

Secrets are never printed. A password is shown as its length and the first
eight hex digits of its SHA-256, which is enough to confirm it matches the
value you expect without putting it on screen:

    printf '%s' 'the-password' | shasum -a 256 | cut -c1-8`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		chosenProfile := profileFlag
		if chosenProfile == "" && flagTLD != "" {
			chosenProfile = flagTLD
		}

		fmt.Fprintf(out, "credentials file:  %s\n", credentialsFilePath())
		fmt.Fprintf(out, "profile:           %s\n", displayOrNone(chosenProfile))

		rec, loadErr := cred.Load(chosenProfile, credentialsFileFlag)
		if loadErr != nil {
			fmt.Fprintf(out, "credentials load:  FAILED: %v\n", loadErr)
		} else {
			fmt.Fprintf(out, "credentials load:  ok (%d keys)\n", len(rec))
		}
		fmt.Fprintln(out)

		cfg, err := buildConfigFromInputs()
		if err != nil {
			fmt.Fprintf(out, "resolved config:   FAILED: %v\n", err)
			return nil
		}

		fmt.Fprintf(out, "tld:               %s\n", cfg.TLD)
		fmt.Fprintf(out, "environment:       %s\n", cfg.Environment)
		fmt.Fprintf(out, "entity:            %s\n", cfg.Entity)
		fmt.Fprintf(out, "api version:       %s\n", cfg.Version)
		fmt.Fprintf(out, "auth type:         %s\n", cfg.AuthType)

		switch cfg.AuthType {
		case base.AUTH_TYPE_BASIC:
			fmt.Fprintf(out, "username:          %s%s\n", displayOrNone(cfg.Username), sourceOf(flagUser, rec["username"]))
			fmt.Fprintf(out, "password:          %s%s\n", describeSecret(cfg.Password), sourceOf(flagPass, rec["password"]))
			if warn := suspiciousSecret(cfg.Password); warn != "" {
				fmt.Fprintf(out, "                   note: %s\n", warn)
			}
		case base.AUTH_TYPE_TLSA:
			fmt.Fprintf(out, "certificate_pem:   %s\n", describePEM(cfg.CertificatePEM))
			fmt.Fprintf(out, "key_pem:           %s\n", describePEM(cfg.KeyPEM))
			fmt.Fprintf(out, "key_passphrase:    %s\n", describeSecret(cfg.KeyPassphrase))
		}

		fmt.Fprintln(out)
		rriBase := base.RRI_URL
		mosBase := base.MOSAPI_URL
		if cfg.Environment == base.ENV_OTE {
			rriBase, mosBase = base.RRI_OTE_URL, base.MOSAPI_OTE_URL
		}
		fmt.Fprintf(out, "RRI base url:      %s\n", rriBase)
		fmt.Fprintf(out, "MOSAPI base url:   %s\n", mosBase)
		fmt.Fprintf(out, "escrow submit url: %s/report/registry-escrow-report/%s/<id>\n", rriBase, cfg.TLD)
		return nil
	},
}

// credentialsFilePath reports which file credentials are read from, mirroring
// the resolution order in the cred package.
func credentialsFilePath() string {
	if credentialsFileFlag != "" {
		return credentialsFileFlag + "  (--credentials-file)"
	}
	if v := os.Getenv("ICANN_SHARED_CREDENTIALS_FILE"); v != "" {
		return v + "  (ICANN_SHARED_CREDENTIALS_FILE)"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".icann", "credentials") + "  (default)"
}

// describeSecret renders a secret as its length and a short SHA-256 prefix, so
// it can be compared against a known value without being displayed.
func describeSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("set, length %d, sha256:%s", len(s), hex.EncodeToString(sum[:])[:8])
}

// describePEM summarises a PEM block without printing key material.
func describePEM(s string) string {
	if s == "" {
		return "(not set)"
	}
	first := s
	if i := strings.Index(s, `\n`); i >= 0 {
		first = s[:i]
	} else if i := strings.IndexByte(s, '\n'); i >= 0 {
		first = s[:i]
	}
	return fmt.Sprintf("set, length %d, starts %q", len(s), first)
}

// sourceOf reports where a value came from, which disambiguates a flag
// overriding the credentials file.
func sourceOf(fromFlag, fromFile string) string {
	switch {
	case fromFlag != "":
		return "  (from flag)"
	case fromFile != "":
		return "  (from credentials file)"
	default:
		return ""
	}
}

// suspiciousSecret flags shapes that indicate the value was mangled on the way
// in, most often by a parser treating part of it as a comment.
func suspiciousSecret(s string) string {
	if s == "" {
		return ""
	}
	var notes []string
	if strings.ContainsAny(s, "#;") {
		notes = append(notes, `contains "#" or ";" — correct only on a build that reads credentials verbatim`)
	}
	if strings.HasPrefix(s, `"`) || strings.HasSuffix(s, `"`) {
		notes = append(notes, "starts or ends with a quote, which usually means the quotes were not stripped")
	}
	if strings.TrimFunc(s, unicode.IsSpace) != s {
		notes = append(notes, "has leading or trailing whitespace")
	}
	return strings.Join(notes, "; ")
}

func displayOrNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
}
