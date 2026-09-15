package rootcmd

import (
	"fmt"
	"time"

	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var rriMonthlyCmd = &cobra.Command{
	Use:   "monthly",
	Short: "Specification 3 monthly report operations",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
}

var rriMonthlyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether ICANN holds a monthly report for a month",
	Long: "Check whether ICANN holds a Specification 3 monthly report for the given month.\n\n" +
		"--type is required: unlike `icann submit monthly`, there is no file here to detect it from.\n" +
		"--month defaults to the previous complete month, since ICANN never accepts the current one.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		typ, err := parseReportTypeFlag(flagReportType)
		if err != nil {
			return err
		}
		if typ == "" {
			return fmt.Errorf("--type is required and must be %q or %q", rri.ReportTransactions, rri.ReportActivity)
		}

		month := flagMonth
		if month == "" {
			month = previousMonth(time.Now())
		} else if !monthlyMonthPattern(month) {
			return fmt.Errorf("--month %q must be in YYYY-MM form", month)
		}

		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := newRRIClient(cfg)
		if err != nil {
			return err
		}

		out, err := cli.GetMonthlyReportStatus(cmd.Context(), typ, month)
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), out)
	},
}

// previousMonth returns the YYYY-MM month before now. ICANN accepts a monthly
// report only once the month has ended, so the current month is never a useful
// default.
func previousMonth(now time.Time) string {
	return now.AddDate(0, 0, -now.Day()).Format("2006-01")
}

func init() {
	getCmd.AddCommand(rriMonthlyCmd)
	rriMonthlyCmd.AddCommand(rriMonthlyStatusCmd)

	f := rriMonthlyStatusCmd.Flags()
	f.StringVar(&flagReportType, "type", "", "Report type: transactions or activity (required)")
	f.StringVar(&flagMonth, "month", "", "Month in YYYY-MM form (default: the previous complete month)")
}
