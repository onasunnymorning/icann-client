package rootcmd

import (
	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/spf13/cobra"
)

var tldStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get TLD SLA monitoring status",
	Long: "Show ICANN's SLA monitoring state for the TLD: whether DNS, RDDS/RDAP and,\n" +
		"where applicable, EPP are currently up, and any incidents counting toward\n" +
		"the emergency-threshold downtime budget for each.",
	Example: "  icann get tld status --tld example",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := mosapi.New(cfg)
		if err != nil {
			return err
		}
		sr, err := cli.GetStateResponse(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), sr)
	},
}

func init() { tldCmd.AddCommand(tldStatusCmd) }
