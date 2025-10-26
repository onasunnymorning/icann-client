package rootcmd

import (
	"encoding/json"
	"fmt"
	"os"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/cmd/icann/internal/cred"
	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/spf13/cobra"
)

var (
	flagStartDate string
	flagEndDate   string
)

var metricaCmd = &cobra.Command{
	Use:   "metrica",
	Short: "Domain METRICA reports",
}

var metricaLatestCmd = &cobra.Command{
	Use:   "latest",
	Short: "Get latest METRICA domain list report",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := mosapi.New(cfg)
		if err != nil {
			return err
		}
		out, err := cli.GetMetricaLatest(cmd.Context())
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

var metricaDateCmd = &cobra.Command{
	Use:   "date <YYYY-MM-DD>",
	Short: "Get METRICA domain list report for a date",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		date := args[0]
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := mosapi.New(cfg)
		if err != nil {
			return err
		}
		out, err := cli.GetMetricaByDate(cmd.Context(), date)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

var metricaListsCmd = &cobra.Command{
	Use:   "lists",
	Short: "List available METRICA reports",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := mosapi.New(cfg)
		if err != nil {
			return err
		}
		out, err := cli.ListMetricaReports(cmd.Context(), flagStartDate, flagEndDate)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

// buildConfigFromInputs consolidates flags and credentials resolution (shared with state command pattern)
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
	}

	if cfg.TLD == "" {
		return base.Config{}, fmt.Errorf("tld is required (provide --tld, credentials tld, or use a profile named after the TLD)")
	}
	if err := cfg.Validate(); err != nil {
		return base.Config{}, err
	}
	return cfg, nil
}

func init() {
	getCmd.AddCommand(metricaCmd)
	metricaCmd.AddCommand(metricaLatestCmd)
	metricaCmd.AddCommand(metricaDateCmd)
	metricaCmd.AddCommand(metricaListsCmd)

	// Reuse global flags for auth/env/tld/etc. Add METRICA-specific flags
	metricaListsCmd.Flags().StringVar(&flagStartDate, "start-date", "", "Filter: start date (YYYY-MM-DD)")
	metricaListsCmd.Flags().StringVar(&flagEndDate, "end-date", "", "Filter: end date (YYYY-MM-DD)")
}
