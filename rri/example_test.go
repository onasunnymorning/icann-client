package rri_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

func ExampleClient_GetRyEscrowReportStatus() {
	// Fake RRI escrow status endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info/report/registry-escrow-report/example/2025-10-22" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	rc, _ := rri.New(cfg)
	_ = rc.WithBaseURL(srv.URL)
	st, _ := rc.GetRyEscrowReportStatus(context.Background(), time.Date(2025, 10, 22, 0, 0, 0, 0, time.UTC))
	fmt.Println(st.Status)
	// Output: received
}

func ExampleClient_SubmitRyEscrowReport() {
	// Fake RRI escrow report submission endpoint. Note the path has no /info/
	// prefix: that belongs to the status endpoint only.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/report/registry-escrow-report/example/example-20250101-full" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0">` +
			`<result code="1000"><msg>accepted</msg></result></response>`))
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	rc, _ := rri.New(cfg)
	_ = rc.WithBaseURL(srv.URL)

	report, _ := os.ReadFile(filepath.Join("testdata", "report-full-prefixed.xml"))

	// The id comes out of the report itself and must match the one in the URL.
	meta, _ := rri.ParseRyEscrowReport(report)

	// The report is sent verbatim; a rejection would come back as a
	// *rri.ResultError, which can arrive with HTTP 200.
	res, err := rc.SubmitRyEscrowReport(context.Background(), meta.ID, report)
	if err != nil {
		if code, ok := rri.ResultCodeOf(err); ok {
			fmt.Println("rejected with result code", code)
			return
		}
		fmt.Println("failed:", err)
		return
	}
	fmt.Println(res.ID, res.ResultCode)
	// Output: example-20250101-full 1000
}

func ExampleClient_SubmitMonthlyReport() {
	// Fake Specification 3 monthly reporting endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/report/registrar-transactions/example/2025-01" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0">` +
			`<result code="1000"><msg>accepted</msg></result></response>`))
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	rc, _ := rri.New(cfg)
	_ = rc.WithBaseURL(srv.URL)

	name := "example-transactions-202501.csv"
	report, _ := os.ReadFile(filepath.Join("testdata", name))

	// Specification 3 names the file after its TLD, type and month, so both the
	// endpoint and the month in the URL can be derived from the filename.
	month, typ := rri.ParseMonthlyFilename(name)

	// Pre-flight locally first: a wrong totals line is otherwise invisible
	// until ICANN rejects it, and every request is rate-limited.
	meta, _ := rri.ParseMonthlyReport(report)
	if err := meta.Validate(rri.MonthlyValidationOptions{TLD: "example", Month: month, Type: typ}); err != nil {
		fmt.Println("would be rejected:", err)
		return
	}

	// The CSV is sent verbatim; a rejection comes back as a *rri.ResultError,
	// which can arrive with HTTP 200.
	res, err := rc.SubmitMonthlyReport(context.Background(), typ, month, report)
	if err != nil {
		if code, ok := rri.ResultCodeOf(err); ok {
			fmt.Println("rejected with result code", code, "-", rri.ResultHint(code))
			return
		}
		fmt.Println("failed:", err)
		return
	}
	fmt.Println(res.Type, res.Month, res.ResultCode)
	// Output: transactions 2025-01 1000
}

func ExampleClient_GetReportingStatus() {
	// Fake RRI reporting status endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info/status/registry/example" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rriReporting:summary
  xmlns:rriReporting="urn:ietf:params:xml:ns:rriReporting-1.0"
  xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeHeader:tld>example</rdeHeader:tld>
  <rriReporting:statusReports>
    <rriReporting:statusReport>
      <rriReporting:type>Registry_Functions_Activity_Report</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>unsatisfactory</rriReporting:status>
      <rriReporting:issues>
        <rriReporting:issue date="2026-08-01" description="No_Report_Received" />
      </rriReporting:issues>
    </rriReporting:statusReport>
  </rriReporting:statusReports>
</rriReporting:summary>`)
	}))
	defer srv.Close()

	c, _ := rri.New(base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"})
	_ = c.WithBaseURL(srv.URL)

	summary, err := c.GetReportingStatus(context.Background())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, r := range summary.Unsatisfactory() {
		for _, issue := range r.Issues {
			fmt.Printf("%s: %s on %s\n", r.Type, issue.Description, issue.Date)
		}
	}
	// Output: Registry_Functions_Activity_Report: No_Report_Received on 2026-08-01
}
