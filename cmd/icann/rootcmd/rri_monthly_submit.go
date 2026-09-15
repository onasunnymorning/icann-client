package rootcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var (
	flagMonth      string
	flagReportType string
)

// maxMonthlySize guards against submitting the wrong file entirely. A monthly
// report is one line per registrar at most, never tens of megabytes.
const maxMonthlySize = 32 << 20

var submitMonthlyCmd = &cobra.Command{
	Use:   "monthly <file|dir|glob>...",
	Short: "Submit Specification 3 monthly reports (transactions, activity) to ICANN",
	Long: `Submit Per-Registrar Transactions and Registry Functions Activity reports to ICANN.

Each argument may be a report file, a directory (its *.csv entries are submitted
in lexical order), or a glob. A single run may mix both report types and several
months: each file's type is detected from its CSV header line and its month is
read from the filename, which Specification 3 requires to be

    <tld>-transactions-<yyyymm>.csv
    <tld>-activity-<yyyymm>.csv

Use --type and --month to override that for a single file. Every file is read
and validated before the first request is sent, so a bad file late in the batch
fails the run rather than the ninth upload. Reports are then submitted
sequentially over a single connection: ICANN rate-limits on authentication, so
--delay defaults to 1s and the batch deliberately does not parallelise.

Before a month's cut-off date a report may be replaced as many times as needed,
so re-running a partial backfill is safe. After the cut-off ICANN rejects the
replacement with result code 2002, which no client can work around.`,
	Example: "  icann submit monthly example-transactions-202605.csv --tld example\n" +
		"  icann submit monthly ./reports/ --tld example --dry-run\n" +
		"  icann submit monthly report.csv --tld example --type activity --month 2026-05",
	Args: cobra.MinimumNArgs(1),
	RunE: runSubmitMonthly,
}

func runSubmitMonthly(cmd *cobra.Command, args []string) error {
	files, err := expandReportPaths(args, ".csv")
	if err != nil {
		return err
	}

	forced, err := parseReportTypeFlag(flagReportType)
	if err != nil {
		return err
	}
	if flagMonth != "" {
		if len(files) > 1 {
			return fmt.Errorf("--month applies to a single report, but %d files were selected", len(files))
		}
		if !monthlyMonthPattern(flagMonth) {
			return fmt.Errorf("--month %q must be in YYYY-MM form", flagMonth)
		}
	}
	if flagNoPreflight {
		if flagMonth == "" || forced == "" {
			return errors.New("--no-preflight requires --month and --type, since the report is not parsed to discover them")
		}
		if len(files) > 1 {
			return fmt.Errorf("--no-preflight applies to a single report, but %d files were selected", len(files))
		}
	}

	cfg, err := buildConfigFromInputs()
	if err != nil {
		return err
	}

	// Read and validate everything before spending a single request.
	prepared, err := prepareMonthlySubmissions(files, cfg.TLD, forced)
	if err != nil {
		return err
	}

	// One client for the whole batch: each rri.New builds its own transport and
	// connection pool, so a client per file would mean a TLS handshake and a
	// fresh authentication per report.
	cli, err := newRRIClient(cfg)
	if err != nil {
		return err
	}

	report := submissionReport{TLD: cfg.TLD, DryRun: flagDryRun, Total: len(prepared)}

	for i, p := range prepared {
		if i > 0 && flagDelay > 0 && !flagDryRun {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(flagDelay):
			}
		}

		res := submitOneMonthly(cmd, cli, p)
		switch res.Status {
		case statusAccepted, statusValidated:
			report.Succeeded++
		case statusSkipped:
			report.Skipped++
		default:
			report.Failed++
		}
		report.Results = append(report.Results, res)

		fmt.Fprintln(cmd.ErrOrStderr(), progressLine(res))

		if flagStopOnError && report.Failed > 0 {
			break
		}
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	if report.Failed > 0 {
		return fmt.Errorf("%d of %d submissions failed", report.Failed, report.Total)
	}
	return nil
}

// preparedMonthly is a monthly report that has been read and locally validated.
type preparedMonthly struct {
	file  string
	typ   rri.ReportType
	month string
	body  []byte
}

// prepareMonthlySubmissions reads, identifies and validates every report up
// front. It fails on the first bad file, before any request has been made.
func prepareMonthlySubmissions(files []string, tld string, forced rri.ReportType) ([]preparedMonthly, error) {
	prepared := make([]preparedMonthly, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		if info.Size() > maxMonthlySize {
			return nil, fmt.Errorf("%s is %d bytes; that is far larger than a monthly report should be", f, info.Size())
		}
		body, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("%s is empty", f)
		}

		nameMonth, nameType := rri.ParseMonthlyFilename(filepath.Base(f))

		p := preparedMonthly{file: f, body: body, typ: forced, month: flagMonth}
		if flagNoPreflight {
			prepared = append(prepared, p)
			continue
		}

		meta, err := rri.ParseMonthlyReport(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}

		// A file named ...-activity-... whose header says transactions is the
		// likeliest way a backfill files the wrong report against the wrong
		// endpoint, so a disagreement is an error rather than a preference.
		if nameType != "" && meta.Type != "" && nameType != meta.Type {
			return nil, fmt.Errorf("%s: the filename says this is a %s report but its CSV header line is a %s report; rename the file or pass --type", f, nameType, meta.Type)
		}

		if p.typ == "" {
			switch {
			case meta.Type != "":
				p.typ = meta.Type
			case nameType != "":
				p.typ = nameType
			default:
				return nil, fmt.Errorf("%s: %w from its columns (%s) or its filename; pass --type transactions|activity", f, rri.ErrMonthlyTypeUnknown, strings.Join(meta.Columns, ","))
			}
		}
		if p.month == "" {
			if nameMonth == "" {
				return nil, fmt.Errorf("%s: %w; the filename must contain a YYYYMM or YYYY-MM month, as in example-transactions-202501.csv or registrar-transactions-2025-01.csv, or pass --month YYYY-MM", f, rri.ErrMonthlyMonthMissing)
			}
			p.month = nameMonth
		}

		if err := meta.Validate(rri.MonthlyValidationOptions{TLD: tld, Month: p.month, Type: p.typ}); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		prepared = append(prepared, p)
	}
	return prepared, nil
}

// submitOneMonthly submits a single prepared report and classifies the outcome.
func submitOneMonthly(cmd *cobra.Command, cli *rri.Client, p preparedMonthly) submissionResult {
	res := submissionResult{File: p.file, Type: string(p.typ), Month: p.month}

	if flagSkipReceived {
		st, err := cli.GetMonthlyReportStatus(cmd.Context(), p.typ, p.month)
		if err != nil {
			res.Status = statusError
			res.Error = fmt.Sprintf("checking existing status: %v", err)
			return res
		}
		if st.Status == rri.RY_RDEReport_RECEIVED {
			res.Status = statusSkipped
			res.Message = "a report of this type for this month has already been received"
			return res
		}
	}

	if flagDryRun {
		res.Status = statusValidated
		res.Message = "not submitted (--dry-run)"
		return res
	}

	out, err := cli.SubmitMonthlyReport(cmd.Context(), p.typ, p.month, p.body)
	if err != nil {
		classifySubmitError(&res, err)
		return res
	}

	res.Status = statusAccepted
	res.URL = out.URL
	res.HTTPStatus = out.HTTPStatus
	res.ResultCode = out.ResultCode
	res.Message = out.Message
	return res
}

// parseReportTypeFlag resolves the --type flag. An empty value means the type
// is detected per file.
func parseReportTypeFlag(v string) (rri.ReportType, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case string(rri.ReportTransactions):
		return rri.ReportTransactions, nil
	case string(rri.ReportActivity):
		return rri.ReportActivity, nil
	default:
		return "", fmt.Errorf("--type %q must be %q or %q", v, rri.ReportTransactions, rri.ReportActivity)
	}
}

// monthlyMonthPattern reports whether v is a YYYY-MM month.
func monthlyMonthPattern(v string) bool {
	_, err := time.Parse("2006-01", v)
	return err == nil && len(v) == 7
}

func init() {
	submitCmd.AddCommand(submitMonthlyCmd)

	f := submitMonthlyCmd.Flags()
	f.StringVar(&flagMonth, "month", "", "Month to submit for in YYYY-MM form (default: read from the filename)")
	f.StringVar(&flagReportType, "type", "", "Force the report type: transactions or activity (default: detected from the CSV header)")
	f.BoolVar(&flagDryRun, "dry-run", false, "Validate locally and report what would be sent, without submitting")
	f.BoolVar(&flagStopOnError, "stop-on-error", false, "Stop at the first failed submission instead of continuing")
	f.DurationVar(&flagDelay, "delay", time.Second, "Delay between submissions; ICANN rate-limits on authentication (0 disables)")
	f.BoolVar(&flagNoPreflight, "no-preflight", false, "Skip local CSV parsing and validation (requires --month, --type and a single file)")
	f.BoolVar(&flagSkipReceived, "skip-received", false, "Check each report's month first and skip if already received (doubles the request count)")
}
