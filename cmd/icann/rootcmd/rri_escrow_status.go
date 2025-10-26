package rootcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var (
	flagDate string
)

var rriEscrowCmd = &cobra.Command{
	Use:   "escrow",
	Short: "Registry escrow operations",
}

var rriEscrowStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Ry Escrow report status for a date",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDate == "" {
			return fmt.Errorf("--date is required (YYYY-MM-DD)")
		}
		dt, err := time.Parse("2006-01-02", flagDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

func init() {
	getCmd.AddCommand(rriEscrowCmd)
	rriEscrowCmd.AddCommand(rriEscrowStatusCmd)

	rriEscrowStatusCmd.Flags().StringVar(&flagDate, "date", "", "Report date (YYYY-MM-DD)")
}
