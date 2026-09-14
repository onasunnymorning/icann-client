package rootcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

const (
	transactionsCSV = "tld,registrar-name,iana-id,total-domains,net-adds-1-yr\n" +
		"example,Registrar Alpha,1001,120,10\n" +
		"example,Registrar Beta,1002,80,7\n" +
		"Totals,,,200,17\n"

	activityCSV = "tld,operational-registrars,whois-43-queries,dns-udp-queries-received\n" +
		"example,3,10000,5000000\n"
)

// writeMonthlyReports creates a directory holding both report types for the
// given months, named per the Specification 3 convention.
func writeMonthlyReports(t *testing.T, months ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, m := range months {
		write(t, filepath.Join(dir, fmt.Sprintf("example-transactions-%s.csv", m)), transactionsCSV)
		write(t, filepath.Join(dir, fmt.Sprintf("example-activity-%s.csv", m)), activityCSV)
	}
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runSubmitMonthlyCmd executes the monthly submit command against a stub server.
func runSubmitMonthlyCmd(t *testing.T, args []string, handler http.HandlerFunc) (submissionReport, string, error) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	prev := newRRIClient
	newRRIClient = func(cfg base.Config) (*rri.Client, error) {
		c, err := rri.New(cfg)
		if err != nil {
			return nil, err
		}
		return c, c.WithBaseURL(srv.URL)
	}
	t.Cleanup(func() { newRRIClient = prev })
	t.Cleanup(resetSubmitFlags)
	resetSubmitFlags()

	var stdout, stderr bytes.Buffer
	cmd := submitMonthlyCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	// Cobra populates the context during Execute; RunE is called directly here.
	cmd.SetContext(t.Context())
	t.Cleanup(func() { cmd.SetOut(nil); cmd.SetErr(nil) })

	full := append([]string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p", "--delay", "0"}, args...)
	if err := cmd.ParseFlags(full); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	runErr := cmd.RunE(cmd, cmd.Flags().Args())

	var report submissionReport
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
			t.Fatalf("decoding stdout %q: %v", stdout.String(), err)
		}
	}
	return report, stderr.String(), runErr
}

// TestSubmitMonthlyBatchMixed is the backfill case: one directory holding both
// report types across two months goes out in one run, over one connection, to
// the right endpoint and month for each file.
func TestSubmitMonthlyBatchMixed(t *testing.T) {
	dir := writeMonthlyReports(t, "202501", "202502")

	var mu sync.Mutex
	var paths []string

	report, stderr, err := runSubmitMonthlyCmd(t, []string{dir}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})

	if err != nil {
		t.Fatalf("run error = %v (stderr: %s)", err, stderr)
	}
	if report.Total != 4 || report.Succeeded != 4 || report.Failed != 0 {
		t.Fatalf("report = %+v, want 4 total and 4 succeeded", report)
	}

	// Lexical order within the directory: activity sorts before transactions.
	want := []string{
		"/report/registry-functions-activity/example/2025-01",
		"/report/registry-functions-activity/example/2025-02",
		"/report/registrar-transactions/example/2025-01",
		"/report/registrar-transactions/example/2025-02",
	}
	if len(paths) != len(want) {
		t.Fatalf("server saw %d requests (%v), want %d", len(paths), paths, len(want))
	}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("request %d path = %q, want %q", i+1, paths[i], w)
		}
	}

	for _, r := range report.Results {
		if r.Type == "" || r.Month == "" {
			t.Errorf("result %+v is missing its type or month", r)
		}
	}
}

// TestSubmitMonthlyUsesOneConnection guards the rate-limit requirement at the
// command level: one client, one connection, for the whole batch.
func TestSubmitMonthlyUsesOneConnection(t *testing.T) {
	dir := writeMonthlyReports(t, "202501", "202502")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	}))
	t.Cleanup(srv.Close)

	var mu sync.Mutex
	var newClients, fresh int

	prev := newRRIClient
	newRRIClient = func(cfg base.Config) (*rri.Client, error) {
		mu.Lock()
		newClients++
		mu.Unlock()
		c, err := rri.New(cfg)
		if err != nil {
			return nil, err
		}
		if err := c.WithBaseURL(srv.URL); err != nil {
			return nil, err
		}
		return c, nil
	}
	t.Cleanup(func() { newRRIClient = prev })
	t.Cleanup(resetSubmitFlags)
	resetSubmitFlags()

	var stdout, stderr bytes.Buffer
	cmd := submitMonthlyCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetContext(httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if !info.Reused {
				mu.Lock()
				fresh++
				mu.Unlock()
			}
		},
	}))
	t.Cleanup(func() { cmd.SetOut(nil); cmd.SetErr(nil) })

	full := []string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p", "--delay", "0", dir}
	if err := cmd.ParseFlags(full); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := cmd.RunE(cmd, cmd.Flags().Args()); err != nil {
		t.Fatalf("run error = %v (stderr: %s)", err, stderr.String())
	}

	if newClients != 1 {
		t.Errorf("built %d clients, want exactly 1 for the whole batch", newClients)
	}
	if fresh != 1 {
		t.Errorf("opened %d connections for 4 reports, want 1", fresh)
	}
}

// TestSubmitMonthlyTypeConflict is the guard against filing a report against
// the wrong endpoint: the filename and the CSV header must agree.
func TestSubmitMonthlyTypeConflict(t *testing.T) {
	dir := t.TempDir()
	// Transactions content under an activity filename.
	path := filepath.Join(dir, "example-activity-202501.csv")
	write(t, path, transactionsCSV)

	var requests int
	_, _, err := runSubmitMonthlyCmd(t, []string{path}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})
	if err == nil {
		t.Fatal("run succeeded, want an error about the type conflict")
	}
	if !strings.Contains(err.Error(), "filename") {
		t.Errorf("error = %q, want it to name the filename/header disagreement", err)
	}
	if requests != 0 {
		t.Errorf("server saw %d requests, want 0: the conflict must be caught before any request", requests)
	}
}

// TestSubmitMonthlyBadTotalsFailsBeforeAnyRequest is the point of recomputing
// the totals: a wrong totals line costs no authenticated round trip.
func TestSubmitMonthlyBadTotalsFailsBeforeAnyRequest(t *testing.T) {
	dir := writeMonthlyReports(t, "202501")
	// Break the totals line of the transactions report: 17 -> 15.
	bad := strings.Replace(transactionsCSV, "Totals,,,200,17", "Totals,,,200,15", 1)
	write(t, filepath.Join(dir, "example-transactions-202501.csv"), bad)

	var requests int
	_, _, err := runSubmitMonthlyCmd(t, []string{dir}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})
	if err == nil {
		t.Fatal("run succeeded, want a totals error")
	}
	if !strings.Contains(err.Error(), "net-adds-1-yr") {
		t.Errorf("error = %q, want it to name the offending column", err)
	}
	if requests != 0 {
		t.Errorf("server saw %d requests, want 0: the good activity file must not be sent either", requests)
	}
}

// TestSubmitMonthlyRejection checks that a rejection carrying a result code is
// reported as such, with the hint that makes 2002 actionable.
func TestSubmitMonthlyRejection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example-activity-202501.csv")
	write(t, path, activityCSV)

	report, stderr, err := runSubmitMonthlyCmd(t, []string{path}, func(w http.ResponseWriter, r *http.Request) {
		// A rejection on HTTP 200, the trap this client exists to avoid.
		respondCode(w, http.StatusOK, rri.ResultReportExistsCutOff, "cut-off date passed")
	})
	if err == nil {
		t.Fatal("run succeeded, want a non-nil error so the exit code is 1")
	}
	if report.Failed != 1 || report.Succeeded != 0 {
		t.Fatalf("report = %+v, want 1 failed", report)
	}
	got := report.Results[0]
	if got.Status != statusRejected {
		t.Errorf("status = %q, want %q", got.Status, statusRejected)
	}
	if got.ResultCode != rri.ResultReportExistsCutOff {
		t.Errorf("result code = %d, want %d", got.ResultCode, rri.ResultReportExistsCutOff)
	}
	if got.Retryable {
		t.Error("a passed cut-off date is not retryable")
	}
	if !strings.Contains(got.Hint, "cut-off") {
		t.Errorf("hint = %q, want an explanation of the cut-off date", got.Hint)
	}
	if !strings.Contains(stderr, "hint:") {
		t.Errorf("stderr = %q, want the hint surfaced in the progress output", stderr)
	}
}

func TestSubmitMonthlyFlagValidation(t *testing.T) {
	dir := writeMonthlyReports(t, "202501")
	one := filepath.Join(dir, "example-activity-202501.csv")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"month with several files", []string{"--month", "2025-01", dir}, "single report"},
		{"month not YYYY-MM", []string{"--month", "2025-1", one}, "YYYY-MM"},
		{"unknown type", []string{"--type", "escrow", one}, "--type"},
		{"no-preflight without month", []string{"--no-preflight", "--type", "activity", one}, "--no-preflight requires"},
		{"no-preflight without type", []string{"--no-preflight", "--month", "2025-01", one}, "--no-preflight requires"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			_, _, err := runSubmitMonthlyCmd(t, tt.args, func(w http.ResponseWriter, r *http.Request) {
				requests++
				respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
			})
			if err == nil {
				t.Fatal("run succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
			if requests != 0 {
				t.Errorf("server saw %d requests, want 0", requests)
			}
		})
	}
}

// TestSubmitMonthlyOverrides checks that --type and --month drive the URL when
// the filename does not follow the convention.
func TestSubmitMonthlyOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handover-copy.csv")
	write(t, path, activityCSV)

	var gotPath string
	report, _, err := runSubmitMonthlyCmd(t,
		[]string{"--type", "activity", "--month", "2025-03", path},
		func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
		})
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	if want := "/report/registry-functions-activity/example/2025-03"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if report.Succeeded != 1 {
		t.Errorf("report = %+v, want 1 succeeded", report)
	}
}

// TestSubmitMonthlyDryRunMakesNoRequest checks the safest way to inspect a
// hand-off from another provider.
func TestSubmitMonthlyDryRun(t *testing.T) {
	dir := writeMonthlyReports(t, "202501")

	var requests int
	report, _, err := runSubmitMonthlyCmd(t, []string{"--dry-run", dir}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	if requests != 0 {
		t.Errorf("server saw %d requests during --dry-run, want 0", requests)
	}
	if !report.DryRun || report.Succeeded != 2 {
		t.Errorf("report = %+v, want dryRun with 2 validated", report)
	}
	for _, r := range report.Results {
		if r.Status != statusValidated {
			t.Errorf("status = %q, want %q", r.Status, statusValidated)
		}
	}
}

// TestSubmitMonthlyProviderNaming covers the names a migrating provider
// actually hands over: the endpoint name, a hyphenated month, no TLD prefix.
// These must need neither --month nor --type nor a rename.
func TestSubmitMonthlyProviderNaming(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "registrar-transactions-2026-08.csv"), transactionsCSV)
	write(t, filepath.Join(dir, "registry-functions-activity-2026-08.csv"), activityCSV)

	var mu sync.Mutex
	var paths []string
	report, stderr, err := runSubmitMonthlyCmd(t, []string{dir}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})
	if err != nil {
		t.Fatalf("run error = %v (stderr: %s)", err, stderr)
	}
	if report.Succeeded != 2 {
		t.Fatalf("report = %+v, want 2 succeeded", report)
	}
	want := []string{
		"/report/registrar-transactions/example/2026-08",
		"/report/registry-functions-activity/example/2026-08",
	}
	sort.Strings(paths)
	for i, w := range want {
		if i >= len(paths) || paths[i] != w {
			t.Errorf("paths = %v, want %v", paths, want)
			break
		}
	}
}

// TestExpandReportPathsCSV covers the extension parameter added for this
// command; the .xml cases live in the escrow test.
func TestExpandReportPathsCSV(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b-activity-202501.csv"), activityCSV)
	write(t, filepath.Join(dir, "a-activity-202501.csv"), activityCSV)
	write(t, filepath.Join(dir, "notes.txt"), "ignore me")
	write(t, filepath.Join(dir, "report.xml"), "<x/>")

	got, err := expandReportPaths([]string{dir}, ".csv")
	if err != nil {
		t.Fatalf("expandReportPaths() error = %v", err)
	}
	want := []string{
		filepath.Join(dir, "a-activity-202501.csv"),
		filepath.Join(dir, "b-activity-202501.csv"),
	}
	if len(got) != len(want) {
		t.Fatalf("expandReportPaths() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// A directory with no CSV must name the extension it looked for.
	onlyXML := t.TempDir()
	write(t, filepath.Join(onlyXML, "report.xml"), "<x/>")
	if _, err := expandReportPaths([]string{onlyXML}, ".csv"); err == nil {
		t.Error("expandReportPaths() succeeded on a directory with no .csv files")
	} else if !strings.Contains(err.Error(), ".csv") {
		t.Errorf("error = %q, want it to name .csv", err)
	}
}
