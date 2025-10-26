package rootcmd

import (
	"encoding/json"
	"os"

	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/spf13/cobra"
)

var tldStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get TLD monitoring status",
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sr)
	},
}

func init() { tldCmd.AddCommand(tldStatusCmd) }
