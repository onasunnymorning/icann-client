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

var metricaCmd = &cobra.Command{
	Use:   "metrica",
	Short: "Domain METRICA reports",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
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
		return printJSON(cmd.OutOrStdout(), out)
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
		return printJSON(cmd.OutOrStdout(), out)
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
		return printJSON(cmd.OutOrStdout(), out)
	},
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
