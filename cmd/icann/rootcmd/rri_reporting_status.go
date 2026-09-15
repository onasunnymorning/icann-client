package rootcmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var flagIssuesOnly bool

var rriReportingCmd = &cobra.Command{
	Use:   "reporting",
	Short: "ICANN's view of this TLD's reporting obligations",
	// A group, not a command: reject an unknown subcommand instead of
	// silently printing help and exiting 0.
	Args: cobra.NoArgs,
}

var rriReportingStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show which reports ICANN considers outstanding",
	Long: "Show ICANN's own reporting status for this TLD: which obligations are enabled,\n" +
		"whether each is satisfied, and every dated issue recorded against it.\n\n" +
		"This answers \"which reports does ICANN think are missing?\" without submitting anything.\n" +
		"With --issues-only the output is narrowed to the unsatisfactory obligations and the\n" +
		"command exits non-zero when any remain, so it can be used as a check in a script.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := newRRIClient(cfg)
		if err != nil {
			return err
		}

		out, err := cli.GetReportingStatus(cmd.Context())
		if err != nil {
			return err
		}

		if !flagIssuesOnly {
			return printJSON(cmd.OutOrStdout(), out)
		}

		out.Reports = out.Unsatisfactory()
		if err := printJSON(cmd.OutOrStdout(), out); err != nil {
			return err
		}
		if n := len(out.Reports); n > 0 {
			// Silence cobra's usage dump: the report above is the message, and
			// the non-zero exit is the point.
			cmd.SilenceUsage = true
			return fmt.Errorf("%d reporting obligation(s) are unsatisfactory", n)
		}
		return nil
	},
}

var rriConformanceCmd = &cobra.Command{
	Use:   "conformance",
	Short: "Show which RRI specifications ICANN implements",
	Long: "Show which RRI specification versions ICANN implements.\n\n" +
		"ICANN answers HTTP 404 on servers that predate this endpoint, which the draft\n" +
		"defines as conformance to two specific versions. Those are reported with\n" +
		"\"inferred\": true rather than as an error.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}
		cli, err := newRRIClient(cfg)
		if err != nil {
			return err
		}
		out, err := cli.GetConformanceVersion(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), out)
	},
}

func init() {
	getCmd.AddCommand(rriReportingCmd)
	rriReportingCmd.AddCommand(rriReportingStatusCmd)
	getCmd.AddCommand(rriConformanceCmd)

	rriReportingStatusCmd.Flags().BoolVar(&flagIssuesOnly, "issues-only",
		false, "Show only unsatisfactory obligations and exit non-zero if any remain")
}
