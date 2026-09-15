package rootcmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var flagStatusJSON bool

// newMosapiClient builds the MOSAPI client used by `icann status`. It is a
// variable, like newRRIClient, so tests can point it at a stub server.
var newMosapiClient = mosapi.New

// reportingPathLabels translates ICANN's reporting-obligation path constants
// into plain language for `icann status`'s human-readable summary.
var reportingPathLabels = map[string]string{
	rri.ReportingPathFull:     "Full registry escrow deposit",
	rri.ReportingPathDiff:     "Differential registry escrow deposit",
	rri.ReportingPathDea:      "Escrow agent notification (DEA)",
	rri.ReportingPathPRTR:     "Monthly per-registrar transactions report (Specification 3)",
	rri.ReportingPathRFAR:     "Monthly registry-functions activity report (Specification 3)",
	rri.ReportingPathRegistry: "Registry",
}

// statusReport is the combined, at-a-glance view `icann status` prints: SLA
// monitoring from MOSAPI and ICANN's own reporting-obligation status from
// RRI. The two lookups are independent, so either can succeed while the other
// reports an error.
type statusReport struct {
	TLD            string                `json:"tld"`
	SLA            *mosapi.StateResponse `json:"sla,omitempty"`
	SLAError       string                `json:"slaError,omitempty"`
	Reporting      *rri.ReportingSummary `json:"reporting,omitempty"`
	ReportingError string                `json:"reportingError,omitempty"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether this TLD is compliant with ICANN right now",
	Long: `Show a one-shot operational summary for the TLD: SLA/uptime monitoring
status and ICANN's own current view of each reporting obligation (registry
escrow, the Specification 3 monthly reports, and the escrow agent's
notification).

This combines 'icann get tld status' and 'icann get reporting status' into a
single check, so you don't need to already know the command tree to answer
"is this TLD in good standing right now?". Like 'icann get reporting status',
the reporting half is a snapshot, not a history: it says whether each
obligation is currently satisfied, not which periods were ever missed. The
default output is a short human-readable summary; pass --json for the
underlying structured data.

Exits non-zero if SLA monitoring reports the TLD down, any reporting
obligation is unsatisfactory, or either check could not be reached — so it
can be used as a monitoring check.`,
	Example: "  icann status --tld example\n" +
		"  icann status --tld example --json",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfigFromInputs()
		if err != nil {
			return err
		}

		out := statusReport{TLD: cfg.TLD}
		var problems []string

		if mc, err := newMosapiClient(cfg); err != nil {
			out.SLAError = err.Error()
			problems = append(problems, "SLA monitoring: "+err.Error())
		} else if sr, err := mc.GetStateResponse(cmd.Context()); err != nil {
			out.SLAError = err.Error()
			problems = append(problems, "SLA monitoring: "+err.Error())
		} else {
			out.SLA = sr
			if sr.Status == "Down" {
				problems = append(problems, "SLA monitoring reports the TLD down")
			}
		}

		if rc, err := newRRIClient(cfg); err != nil {
			out.ReportingError = err.Error()
			problems = append(problems, "reporting status: "+err.Error())
		} else if rs, err := rc.GetReportingStatus(cmd.Context()); err != nil {
			out.ReportingError = err.Error()
			problems = append(problems, "reporting status: "+err.Error())
		} else {
			out.Reporting = rs
			if n := len(rs.Unsatisfactory()); n > 0 {
				problems = append(problems, fmt.Sprintf("%d reporting obligation(s) are unsatisfactory", n))
			}
		}

		if flagStatusJSON {
			if err := printJSON(cmd.OutOrStdout(), out); err != nil {
				return err
			}
		} else {
			printStatusSummary(cmd.OutOrStdout(), out)
		}

		if len(problems) > 0 {
			// The summary or JSON above is the message; silence cobra's usage
			// dump so the non-zero exit doesn't get buried under it.
			cmd.SilenceUsage = true
			return fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return nil
	},
}

func printStatusSummary(w io.Writer, out statusReport) {
	fmt.Fprintf(w, "TLD: %s\n\n", out.TLD)

	fmt.Fprintln(w, "SLA monitoring:")
	if out.SLAError != "" {
		fmt.Fprintf(w, "  could not check: %s\n", out.SLAError)
	} else {
		fmt.Fprintf(w, "  %s\n", out.SLA.Status)
		var services []string
		for name := range out.SLA.TestedServices {
			services = append(services, name)
		}
		sort.Strings(services)
		for _, name := range services {
			svc := out.SLA.TestedServices[name]
			fmt.Fprintf(w, "    %-6s %s\n", name+":", svc.Status)
			for _, inc := range svc.Incidents {
				if inc.EndTime == nil {
					fmt.Fprintf(w, "      ongoing incident since %s\n", inc.StartTimeTime().Format("2006-01-02 15:04 UTC"))
				}
			}
		}
	}

	fmt.Fprintln(w, "\nReporting obligations (as of ICANN's last snapshot):")
	if out.ReportingError != "" {
		fmt.Fprintf(w, "  could not check: %s\n", out.ReportingError)
	} else {
		for _, p := range out.Reporting.Paths {
			label := reportingPathLabels[p.Path]
			if label == "" {
				label = p.Path
			}
			if p.Status == rri.ReportingStatusOK {
				fmt.Fprintf(w, "  [ok] %s\n", label)
			} else {
				fmt.Fprintf(w, "  [UNSATISFACTORY] %s (status: %s)\n", label, p.Status)
			}
		}
	}
}

func init() {
	RootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&flagStatusJSON, "json", false, "Print the structured JSON result instead of the human summary")
}
