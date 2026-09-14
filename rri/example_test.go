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
