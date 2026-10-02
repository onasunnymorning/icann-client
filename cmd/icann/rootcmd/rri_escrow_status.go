package rootcmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var (
	flagDate    string
	flagDetails bool
)

var rriEscrowCmd = &cobra.Command{
	Use:   "escrow",
	Short: "Check registry escrow deposits and agent notifications",
	Args:  cobra.NoArgs,
	RunE:  requireSubcommand,
}

// dateFlagOrToday parses --date, defaulting to today since "did today's
// deposit land?" is the question these commands exist to answer.
func dateFlagOrToday() (time.Time, error) {
	date := flagDate
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	dt, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, fmt.Errorf("--date %q is not a valid date; use YYYY-MM-DD", date)
	}
	return dt, nil
}

// withHint appends ResultHint to an ICANN rejection, since Execute prints the
// error as is and the bare "result code 2214" says nothing about what to do.
func withHint(err error) error {
	var re *rri.ResultError
	if errors.As(err, &re) {
		if hint := rri.ResultHint(re.Code); hint != "" {
			return fmt.Errorf("%w\nhint: %s", err, hint)
		}
	}
	return err
}

var rriEscrowStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether ICANN holds an escrow deposit for a date",
	Long: "Check whether ICANN holds a registry data escrow deposit for the given date.\n\n" +
		"--date defaults to today, since \"did today's deposit land?\" is the\n" +
		"question this command exists to answer.\n\n" +
		"--details also lists each deposit ICANN holds for the date, with the time ICANN\n" +
		"received it. ICANN serves that only for dates after draft -27 went live in\n" +
		"production; for an earlier date it answers with result code 2214.",
	Example: "  icann get escrow status --tld example\n" +
		"  icann get escrow status --tld example --date 2026-06-01\n" +
		"  icann get escrow status --tld example --details",
	RunE: func(cmd *cobra.Command, args []string) error {
		dt, err := dateFlagOrToday()
		if err != nil {
			return err
		}

		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := newRRIClient(cfg)
		if err != nil {
			return err
		}

		if flagDetails {
			out, err := cli.GetRyEscrowReports(cmd.Context(), dt)
			if err != nil {
				return withHint(err)
			}
			return printJSON(cmd.OutOrStdout(), out)
		}
		out, err := cli.GetRyEscrowReportStatus(cmd.Context(), dt)
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), out)
	},
}

var rriEscrowNotificationsCmd = &cobra.Command{
	Use:   "notifications",
	Short: "List the escrow agent notifications ICANN received for a date",
	Long: "List the data escrow agent notifications ICANN received for the given date, each with\n" +
		"the time ICANN received it. This is how to tell whether your escrow agent has\n" +
		"verified a deposit (status DVPN) or reported none received (DRFN).\n\n" +
		"--date defaults to today. ICANN serves this only for dates after draft -27 went live\n" +
		"in production; for an earlier date it answers with result code 2214.",
	Example: "  icann get escrow notifications --tld example\n" +
		"  icann get escrow notifications --tld example --date 2026-06-01",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dt, err := dateFlagOrToday()
		if err != nil {
			return err
		}

		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := newRRIClient(cfg)
		if err != nil {
			return err
		}

		out, err := cli.GetEscrowNotifications(cmd.Context(), dt)
		if err != nil {
			return withHint(err)
		}
		return printJSON(cmd.OutOrStdout(), out)
	},
}

func init() {
	getCmd.AddCommand(rriEscrowCmd)
	rriEscrowCmd.AddCommand(rriEscrowStatusCmd)
	rriEscrowCmd.AddCommand(rriEscrowNotificationsCmd)

	rriEscrowStatusCmd.Flags().StringVar(&flagDate, "date", "", "Deposit date (YYYY-MM-DD, default: today)")
	rriEscrowStatusCmd.Flags().BoolVar(&flagDetails, "details", false, "List each deposit with the time ICANN received it (needs a date after draft -27 went live)")
	rriEscrowNotificationsCmd.Flags().StringVar(&flagDate, "date", "", "Report date (YYYY-MM-DD, default: today)")
}
