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
	"strings"
	"sync"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

const reportTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<rdeReport:report
  xmlns:rdeReport="urn:ietf:params:xml:ns:rdeReport-1.0"
  xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeReport:id>example-2025010%[1]d-full</rdeReport:id>
  <rdeReport:version>1</rdeReport:version>
  <rdeReport:crDate>2025-01-0%[1]dT09:00:00Z</rdeReport:crDate>
  <rdeReport:kind>FULL</rdeReport:kind>
  <rdeReport:watermark>2025-01-0%[1]dT00:00:00Z</rdeReport:watermark>
  <rdeHeader:header>
    <rdeHeader:tld>example</rdeHeader:tld>
    <rdeHeader:count uri="urn:ietf:params:xml:ns:rdeDomain-1.0">10</rdeHeader:count>
  </rdeHeader:header>
</rdeReport:report>`

// writeReports creates n reports in a fresh directory and returns its path.
func writeReports(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 1; i <= n; i++ {
		name := filepath.Join(dir, fmt.Sprintf("example-2025010%d-full.xml", i))
		if err := os.WriteFile(name, []byte(fmt.Sprintf(reportTemplate, i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runSubmit executes the submit command against a stub server and returns the
// decoded report, the stderr progress output and the command's error.
func runSubmit(t *testing.T, args []string, handler http.HandlerFunc) (submissionReport, string, error) {
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
	cmd := submitEscrowReportCmd
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

// resetSubmitFlags restores the package-level flag state between cases.
func resetSubmitFlags() {
	flagReportID, flagDryRun, flagStopOnError = "", false, false
	flagNoPreflight, flagSkipReceived, flagDelay = false, false, 0
	flagTLD, flagAuth, flagUser, flagPass, flagEnv = "", "", "", "", ""
}

// respondCode writes an IIRDEA result envelope.
func respondCode(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0"><result code="%d"><msg>%s</msg></result></response>`, code, msg)
}

// TestSubmitBatchAccepted covers the happy path for a multi-day backfill and,
// critically, asserts the whole batch travelled over one connection: ICANN
// rate-limits on authentication, so a client per file would be a regression.
func TestSubmitBatchAccepted(t *testing.T) {
	dir := writeReports(t, 3)

	var mu sync.Mutex
	var paths []string
	report, stderr, err := runSubmit(t, []string{dir}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})
	if err != nil {
		t.Fatalf("RunE() error = %v", err)
	}

	if report.Total != 3 || report.Succeeded != 3 || report.Failed != 0 {
		t.Errorf("report = %+v, want 3 total and 3 succeeded", report)
	}
	if len(paths) != 3 {
		t.Fatalf("server saw %d requests, want exactly one per report", len(paths))
	}
	for i, want := range []string{
		"/report/registry-escrow-report/example/example-20250101-full",
		"/report/registry-escrow-report/example/example-20250102-full",
		"/report/registry-escrow-report/example/example-20250103-full",
	} {
		if paths[i] != want {
			t.Errorf("request %d path = %q, want %q (reports must go in date order)", i, paths[i], want)
		}
	}
	for _, r := range report.Results {
		if r.Status != statusAccepted || r.ResultCode != rri.ResultSuccess {
			t.Errorf("result = %+v, want accepted with code 1000", r)
		}
	}
	if !strings.Contains(stderr, "accepted") {
		t.Errorf("stderr = %q, want per-file progress markers", stderr)
	}
}

// TestSubmitBatchContinuesPastRejection checks the default behaviour: a
// rejected report does not abandon the remaining days, but the command still
// exits non-zero.
func TestSubmitBatchContinuesPastRejection(t *testing.T) {
	dir := writeReports(t, 3)

	report, _, err := runSubmit(t, []string{dir}, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "20250102-full") {
			// A rejection arriving with HTTP 200 must still count as a failure.
			respondCode(w, http.StatusOK, rri.ResultUnexpectedDIFF, "differential report not expected")
			return
		}
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})

	if err == nil {
		t.Fatal("RunE() = nil, want a non-nil error so the process exits non-zero")
	}
	if report.Total != 3 || report.Succeeded != 2 || report.Failed != 1 {
		t.Errorf("report = %+v, want 3 total, 2 succeeded, 1 failed", report)
	}
	var rejected *submissionResult
	for i := range report.Results {
		if report.Results[i].Status == statusRejected {
			rejected = &report.Results[i]
		}
	}
	if rejected == nil {
		t.Fatalf("no rejected result in %+v", report.Results)
	}
	if rejected.ResultCode != rri.ResultUnexpectedDIFF {
		t.Errorf("result code = %d, want %d", rejected.ResultCode, rri.ResultUnexpectedDIFF)
	}
	if rejected.HTTPStatus != http.StatusOK {
		t.Errorf("httpStatus = %d; a rejection on HTTP 200 must still be recorded as a failure", rejected.HTTPStatus)
	}
}

func TestSubmitBatchStopOnError(t *testing.T) {
	dir := writeReports(t, 3)

	var requests int
	report, _, err := runSubmit(t, []string{dir, "--stop-on-error"}, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		respondCode(w, http.StatusBadRequest, rri.ResultBadRequest, "schema validation failed")
	})

	if err == nil {
		t.Fatal("RunE() = nil, want an error")
	}
	if requests != 1 {
		t.Errorf("server saw %d requests, want 1 before stopping", requests)
	}
	if len(report.Results) != 1 || report.Failed != 1 {
		t.Errorf("report = %+v, want a single failed result", report)
	}
}

// TestSubmitBatchFailsBeforeAnyRequest checks that a bad file anywhere in the
// batch is caught up front, rather than after earlier reports have already
// consumed authenticated requests.
func TestSubmitBatchFailsBeforeAnyRequest(t *testing.T) {
	dir := writeReports(t, 3)
	bad := filepath.Join(dir, "example-20250104-full.xml")
	if err := os.WriteFile(bad, []byte("<not-a-report/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var requests int
	_, _, err := runSubmit(t, []string{dir}, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	})

	if err == nil {
		t.Fatal("RunE() = nil, want the invalid report to fail the run")
	}
	if requests != 0 {
		t.Errorf("server saw %d requests; validation must complete before anything is submitted", requests)
	}
}

func TestSubmitRejectsIDWithMultipleFiles(t *testing.T) {
	dir := writeReports(t, 2)
	_, _, err := runSubmit(t, []string{dir, "--id", "some-id"}, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request should be made")
	})
	if err == nil || !strings.Contains(err.Error(), "--id applies to a single report") {
		t.Errorf("error = %v, want a complaint about --id with several files", err)
	}
}

func TestSubmitBatchReusesOneConnection(t *testing.T) {
	dir := writeReports(t, 3)

	var mu sync.Mutex
	var conns int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondCode(w, http.StatusOK, rri.ResultSuccess, "accepted")
	}))
	defer srv.Close()

	prev := newRRIClient
	newRRIClient = func(cfg base.Config) (*rri.Client, error) {
		mu.Lock()
		conns++
		mu.Unlock()
		c, err := rri.New(cfg)
		if err != nil {
			return nil, err
		}
		return c, c.WithBaseURL(srv.URL)
	}
	defer func() { newRRIClient = prev }()
	defer resetSubmitFlags()
	resetSubmitFlags()

	var dialed int
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if !info.Reused {
				mu.Lock()
				dialed++
				mu.Unlock()
			}
		},
	}

	cmd := submitEscrowReportCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	defer func() { cmd.SetOut(nil); cmd.SetErr(nil) }()
	cmd.SetContext(httptrace.WithClientTrace(t.Context(), trace))

	if err := cmd.ParseFlags([]string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p", "--delay", "0", dir}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, cmd.Flags().Args()); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}

	if conns != 1 {
		t.Errorf("built %d clients, want exactly 1 for the whole batch", conns)
	}
	if dialed != 1 {
		t.Errorf("opened %d connections for 3 reports, want 1; ICANN rate-limits on authentication", dialed)
	}
}
