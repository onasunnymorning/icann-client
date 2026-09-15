package rootcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

var (
	flagReportID     string
	flagDryRun       bool
	flagStopOnError  bool
	flagDelay        time.Duration
	flagNoPreflight  bool
	flagSkipReceived bool
)

// maxReportSize guards against submitting an escrow deposit by mistake. A
// report is a header and a handful of counts, never megabytes of object data.
const maxReportSize = 8 << 20

// submission statuses reported per file.
const (
	statusAccepted  = "accepted"  // ICANN returned result code 1000
	statusRejected  = "rejected"  // ICANN returned a result code other than 1000
	statusError     = "error"     // local or transport failure; no result code
	statusSkipped   = "skipped"   // already received, with --skip-received
	statusValidated = "validated" // --dry-run: checked locally, never sent
)

// submissionResult is the per-file outcome reported on stdout.
type submissionResult struct {
	File       string     `json:"file"`
	Status     string     `json:"status"`
	Type       string     `json:"type,omitempty"`
	Month      string     `json:"month,omitempty"`
	ID         string     `json:"id,omitempty"`
	Kind       string     `json:"kind,omitempty"`
	Watermark  *time.Time `json:"watermark,omitempty"`
	URL        string     `json:"url,omitempty"`
	HTTPStatus int        `json:"httpStatus,omitempty"`
	ResultCode int        `json:"resultCode,omitempty"`
	Message    string     `json:"message,omitempty"`
	Retryable  bool       `json:"retryable,omitempty"`
	Hint       string     `json:"hint,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// submissionReport is the envelope printed for both single and batch runs.
type submissionReport struct {
	TLD       string             `json:"tld"`
	DryRun    bool               `json:"dryRun"`
	Total     int                `json:"total"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Skipped   int                `json:"skipped"`
	Results   []submissionResult `json:"results"`
}

// newRRIClient builds the client used for a batch. It is a variable so that
// tests can point the command at a stub server.
var newRRIClient = rri.New

var submitEscrowCmd = &cobra.Command{
	Use:   "escrow",
	Short: "Submit registry escrow reports to ICANN",
	Args:  cobra.NoArgs,
	RunE:  requireSubcommand,
}

var submitEscrowReportCmd = &cobra.Command{
	Use:   "report <file|dir|glob>...",
	Short: "Submit RDE (registry escrow) reports to ICANN",
	Long: `Submit one or more RDE reports to ICANN.

Each argument may be a report file, a directory (its *.xml entries are submitted
in lexical order), or a glob. Reports are submitted sequentially over a single
connection: ICANN rate-limits on authentication, so --delay defaults to 1s and
the batch deliberately does not parallelise.

The report id is read from the <rdeReport:id> element of each file. Submitting a
report whose id was already accepted overwrites the previous one, so re-running
a partial backfill is safe.`,
	Example: "  icann submit escrow report deposit.xml --tld example\n" +
		"  icann submit escrow report ./deposits/ --tld example --delay 2s\n" +
		"  icann submit escrow report deposit.xml --tld example --dry-run",
	Args: cobra.MinimumNArgs(1),
	RunE: runSubmitEscrowReport,
}

func runSubmitEscrowReport(cmd *cobra.Command, args []string) error {
	files, err := expandReportPaths(args, ".xml")
	if err != nil {
		return err
	}
	if flagReportID != "" && len(files) > 1 {
		return fmt.Errorf("--id applies to a single report, but %d files were selected", len(files))
	}
	if flagNoPreflight {
		if flagReportID == "" {
			return errors.New("--no-preflight requires --id, since the report is not parsed to discover it")
		}
		if flagSkipReceived {
			return errors.New("--no-preflight cannot be combined with --skip-received, which needs the report's watermark date")
		}
	}

	cfg, err := buildConfigFromInputs()
	if err != nil {
		return err
	}

	// Read and validate everything before spending a single request, so that a
	// bad file late in the batch fails the run rather than the ninth upload.
	prepared, err := prepareSubmissions(files, cfg.TLD)
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

		res := submitOne(cmd, cli, p)
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

// preparedReport is a report that has been read and locally validated.
type preparedReport struct {
	file string
	id   string
	body []byte
	meta *rri.ReportMeta
}

// prepareSubmissions reads and validates every report up front. It fails on the
// first bad file, before any request has been made.
func prepareSubmissions(files []string, tld string) ([]preparedReport, error) {
	prepared := make([]preparedReport, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		if info.Size() > maxReportSize {
			return nil, fmt.Errorf("%s is %d bytes; that looks like an escrow deposit rather than a report", f, info.Size())
		}
		body, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("%s is empty", f)
		}

		p := preparedReport{file: f, body: body, id: flagReportID}
		if flagNoPreflight {
			prepared = append(prepared, p)
			continue
		}

		meta, err := rri.ParseRyEscrowReport(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := meta.Validate(rri.ValidationOptions{TLD: tld, ID: flagReportID}); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		p.meta = meta
		if p.id == "" {
			p.id = meta.ID
		}
		prepared = append(prepared, p)
	}
	return prepared, nil
}

// submitOne submits a single prepared report and classifies the outcome.
func submitOne(cmd *cobra.Command, cli *rri.Client, p preparedReport) submissionResult {
	res := submissionResult{File: p.file, ID: p.id}
	if p.meta != nil {
		res.Kind = p.meta.Kind
		wm := p.meta.Watermark
		res.Watermark = &wm
	}

	if flagSkipReceived {
		st, err := cli.GetRyEscrowReportStatus(cmd.Context(), p.meta.Watermark)
		if err != nil {
			res.Status = statusError
			res.Error = fmt.Sprintf("checking existing status: %v", err)
			return res
		}
		if st.Status == rri.RY_RDEReport_RECEIVED {
			res.Status = statusSkipped
			res.Message = "a report for this date has already been received"
			return res
		}
	}

	if flagDryRun {
		res.Status = statusValidated
		res.Message = "not submitted (--dry-run)"
		return res
	}

	out, err := cli.SubmitRyEscrowReport(cmd.Context(), p.id, p.body)
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

// classifySubmitError records a failed submission on res. A rejection carrying
// an ICANN result code is a "rejected" outcome and is reported with the code
// and, where one exists, a hint on what to do about it; anything else is an
// "error". The two error kinds are alternatives, so the order below matters.
func classifySubmitError(res *submissionResult, err error) {
	res.Error = err.Error()
	res.Retryable = rri.IsRetryable(err)

	var re *rri.ResultError
	if errors.As(err, &re) {
		res.Status = statusRejected
		res.ResultCode = re.Code
		res.Message = re.Msg
		res.HTTPStatus = re.HTTPStatus
		res.URL = re.URL
		res.Hint = rri.ResultHint(re.Code)
		return
	}

	res.Status = statusError
	var he *base.HTTPError
	if errors.As(err, &he) {
		res.HTTPStatus = he.StatusCode
		res.URL = he.URL
	}
}

// progressLine renders the one-line human marker written to stderr, so that
// stdout stays a single JSON document.
func progressLine(r submissionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-9s %s", r.Status, r.File)
	if r.ID != "" {
		fmt.Fprintf(&b, "  id=%s", r.ID)
	}
	if r.Type != "" {
		fmt.Fprintf(&b, "  type=%s", r.Type)
	}
	if r.Month != "" {
		fmt.Fprintf(&b, "  month=%s", r.Month)
	}
	if r.ResultCode != 0 {
		fmt.Fprintf(&b, "  code=%d", r.ResultCode)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "  %s", r.Error)
	}
	if r.Hint != "" {
		fmt.Fprintf(&b, "\n          hint: %s", r.Hint)
	}
	return b.String()
}

// expandReportPaths resolves each argument to report files:
//
//   - a directory yields its entries with extension ext, non-recursively, in
//     lexical order
//   - an existing file yields itself, whatever its extension
//   - anything else containing a glob metacharacter is expanded, and must match
//   - anything else is an error naming the missing path
//
// Results are de-duplicated by cleaned path, preserving first-seen order. POSIX
// shells already expand globs, but directory arguments, quoted patterns and
// Windows cmd.exe all reach us unexpanded.
func expandReportPaths(args []string, ext string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}

	add := func(p string) {
		p = filepath.Clean(p)
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	for _, arg := range args {
		info, err := os.Stat(arg)
		switch {
		case err == nil && info.IsDir():
			entries, err := os.ReadDir(arg)
			if err != nil {
				return nil, err
			}
			var found []string
			for _, e := range entries {
				if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ext) {
					continue
				}
				found = append(found, filepath.Join(arg, e.Name()))
			}
			if len(found) == 0 {
				return nil, fmt.Errorf("directory %s contains no %s reports", arg, ext)
			}
			sort.Strings(found)
			for _, f := range found {
				add(f)
			}

		case err == nil:
			add(arg)

		case strings.ContainsAny(arg, "*?["):
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern %q: %w", arg, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("pattern %q matched no files", arg)
			}
			sort.Strings(matches)
			for _, m := range matches {
				add(m)
			}

		default:
			return nil, err
		}
	}
	return out, nil
}

func init() {
	submitCmd.AddCommand(submitEscrowCmd)
	submitEscrowCmd.AddCommand(submitEscrowReportCmd)

	f := submitEscrowReportCmd.Flags()
	f.StringVar(&flagReportID, "id", "", "Report id to submit under (default: the <rdeReport:id> in the file)")
	f.BoolVar(&flagDryRun, "dry-run", false, "Validate locally and report what would be sent, without submitting")
	f.BoolVar(&flagStopOnError, "stop-on-error", false, "Stop at the first failed submission instead of continuing")
	f.DurationVar(&flagDelay, "delay", time.Second, "Delay between submissions; ICANN rate-limits on authentication (0 disables)")
	f.BoolVar(&flagNoPreflight, "no-preflight", false, "Skip local XML parsing and validation (requires --id and a single file)")
	f.BoolVar(&flagSkipReceived, "skip-received", false, "Check each report's date first and skip if already received (doubles the request count)")
}
