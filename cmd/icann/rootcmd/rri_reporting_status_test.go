package rootcmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
	"github.com/spf13/cobra"
)

// runGetCmd drives one of the read-only RRI commands against a test server and
// returns its raw stdout, the paths the server saw, and the command's error.
func runGetCmd(t *testing.T, cmd *cobra.Command, args []string, h http.HandlerFunc) ([]byte, []string, error) {
	t.Helper()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		h(w, r)
	}))
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

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetContext(t.Context())
	t.Cleanup(func() { cmd.SetOut(nil) })

	full := append([]string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p"}, args...)
	if err := cmd.ParseFlags(full); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	runErr := cmd.RunE(cmd, cmd.Flags().Args())
	return stdout.Bytes(), paths, runErr
}

// summaryFixture is inlined rather than read from rri/testdata, so this test
// does not depend on a relative path reaching into another package.
const summaryFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rriReporting:summary
  xmlns:rriReporting="urn:ietf:params:xml:ns:rriReporting-1.0"
  xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeHeader:tld>example</rdeHeader:tld>
  <rriReporting:depositSchedule>Daily</rriReporting:depositSchedule>
  <rriReporting:statusReports>
    <rriReporting:statusReport>
      <rriReporting:type>DEA_Notification</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>unsatisfactory</rriReporting:status>
      <rriReporting:issues>
        <rriReporting:issue date="2026-01-01" description="No_Report_Received" />
        <rriReporting:issue date="2025-12-30" description="Invalid_Deposit_Full" />
      </rriReporting:issues>
    </rriReporting:statusReport>
    <rriReporting:statusReport>
      <rriReporting:type>Registry_Functions_Activity_Report</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>ok</rriReporting:status>
    </rriReporting:statusReport>
  </rriReporting:statusReports>
</rriReporting:summary>`

func TestReportingStatusCmd(t *testing.T) {
	out, paths, err := runGetCmd(t, rriReportingStatusCmd, nil, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(summaryFixture))
	})
	if err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if want := "GET /info/status/registry/example"; len(paths) != 1 || paths[0] != want {
		t.Errorf("requests = %v, want exactly [%s]", paths, want)
	}

	var got rri.ReportingSummary
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	// Without --issues-only every obligation is reported, satisfied or not.
	if len(got.Reports) != 2 {
		t.Errorf("reports = %d, want both obligations from the fixture", len(got.Reports))
	}
}

// TestReportingStatusIssuesOnly is the form a script uses: the unsatisfactory
// obligations on stdout and a non-zero exit, so a backfill check fails loudly.
func TestReportingStatusIssuesOnly(t *testing.T) {
	out, _, err := runGetCmd(t, rriReportingStatusCmd, []string{"--issues-only"}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(summaryFixture))
	})
	if err == nil {
		t.Fatal("RunE() error = nil, want a non-zero exit while an obligation is unsatisfactory")
	}
	if !strings.Contains(err.Error(), "unsatisfactory") {
		t.Errorf("error = %q, want it to say what is outstanding", err)
	}

	var got rri.ReportingSummary
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	if len(got.Reports) != 1 || got.Reports[0].Type != rri.ReportingDEANotification {
		t.Errorf("reports = %+v, want only the unsatisfactory one", got.Reports)
	}
	if len(got.Reports[0].Issues) != 2 {
		t.Errorf("issues = %+v, want the dates ICANN is missing to survive the filter", got.Reports[0].Issues)
	}
}

// TestReportingStatusIssuesOnlyClean is the other half: a TLD that is square
// with ICANN must exit zero, or the check is useless.
func TestReportingStatusIssuesOnlyClean(t *testing.T) {
	const clean = `<?xml version="1.0" encoding="UTF-8"?>
<rriReporting:summary
  xmlns:rriReporting="urn:ietf:params:xml:ns:rriReporting-1.0"
  xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeHeader:tld>example</rdeHeader:tld>
  <rriReporting:statusReports>
    <rriReporting:statusReport>
      <rriReporting:type>Registry_Escrow_Report</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>ok</rriReporting:status>
    </rriReporting:statusReport>
  </rriReporting:statusReports>
</rriReporting:summary>`

	out, _, err := runGetCmd(t, rriReportingStatusCmd, []string{"--issues-only"}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(clean))
	})
	if err != nil {
		t.Fatalf("RunE() error = %v, want a zero exit for a TLD with no issues", err)
	}
	var got rri.ReportingSummary
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	if len(got.Reports) != 0 {
		t.Errorf("reports = %+v, want none", got.Reports)
	}
}

func TestConformanceCmd(t *testing.T) {
	out, paths, err := runGetCmd(t, rriConformanceCmd, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err != nil {
		t.Fatalf("RunE() error = %v, want the documented 404 answer", err)
	}
	if want := "GET /info/status/conformance-version"; len(paths) != 1 || paths[0] != want {
		t.Errorf("requests = %v, want exactly [%s]", paths, want)
	}
	var got rri.Conformance
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	if !got.Inferred || len(got.Specifications) != 2 {
		t.Errorf("conformance = %+v, want two inferred specifications", got)
	}
}
