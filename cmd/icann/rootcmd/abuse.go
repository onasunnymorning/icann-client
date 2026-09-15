// Package rootcmd provides the root command structure and command grouping for the ICANN CLI.
package rootcmd

import (
	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/spf13/cobra"
)

var (
	flagStartDate string
	flagEndDate   string
)

var abuseCmd = &cobra.Command{
	Use:   "abuse",
	Short: "Domain-abuse reports for the TLD",
	Long: `Domain-abuse reports for the TLD, as measured by ICANN's own monitoring
(known there as METRICA, formerly DAAR): a periodic list of domains ICANN
considers abused, broken down by threat type.`,
	Args: cobra.NoArgs,
	RunE: requireSubcommand,
}

var abuseLatestCmd = &cobra.Command{
	Use:     "latest",
	Short:   "Get the most recent domain-abuse report",
	Example: "  icann get abuse latest --tld example",
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
		return printJSON(cmd.OutOrStdout(), out)
	},
}

var abuseDateCmd = &cobra.Command{
	Use:     "date <YYYY-MM-DD>",
	Short:   "Get the domain-abuse report for a specific date",
	Example: "  icann get abuse date 2026-06-01 --tld example",
	Args:    cobra.ExactArgs(1),
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
		return printJSON(cmd.OutOrStdout(), out)
	},
}

var abuseListsCmd = &cobra.Command{
	Use:     "lists",
	Short:   "List the domain-abuse reports ICANN has available",
	Example: "  icann get abuse lists --tld example --start-date 2026-01-01 --end-date 2026-06-01",
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
		return printJSON(cmd.OutOrStdout(), out)
	},
}

func init() {
	getCmd.AddCommand(abuseCmd)
	abuseCmd.AddCommand(abuseLatestCmd)
	abuseCmd.AddCommand(abuseDateCmd)
	abuseCmd.AddCommand(abuseListsCmd)

	// Reuse global flags for auth/env/tld/etc. Add abuse-report-specific flags.
	abuseListsCmd.Flags().StringVar(&flagStartDate, "start-date", "", "Filter: start date (YYYY-MM-DD)")
	abuseListsCmd.Flags().StringVar(&flagEndDate, "end-date", "", "Filter: end date (YYYY-MM-DD)")
}
