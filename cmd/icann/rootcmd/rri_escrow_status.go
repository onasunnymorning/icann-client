package rootcmd

import (
	"fmt"
	"time"

	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var (
	flagDate string
)

var rriEscrowCmd = &cobra.Command{
	Use:   "escrow",
	Short: "Check registry escrow deposit status",
	Args:  cobra.NoArgs,
	RunE:  requireSubcommand,
}

var rriEscrowStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether ICANN holds an escrow deposit for a date",
	Long: "Check whether ICANN holds a registry data escrow deposit for the given date.\n\n" +
		"--date defaults to today, since \"did today's deposit land?\" is the\n" +
		"question this command exists to answer.",
	Example: "  icann get escrow status --tld example\n" +
		"  icann get escrow status --tld example --date 2026-06-01",
	RunE: func(cmd *cobra.Command, args []string) error {
		date := flagDate
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		dt, err := time.Parse("2006-01-02", date)
		if err != nil {
			return fmt.Errorf("--date %q is not a valid date; use YYYY-MM-DD", date)
		}

		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := rri.New(cfg)
		if err != nil {
			return err
		}

		out, err := cli.GetRyEscrowReportStatus(cmd.Context(), dt)
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), out)
	},
}

func init() {
	getCmd.AddCommand(rriEscrowCmd)
	rriEscrowCmd.AddCommand(rriEscrowStatusCmd)

	rriEscrowStatusCmd.Flags().StringVar(&flagDate, "date", "", "Deposit date (YYYY-MM-DD, default: today)")
}
