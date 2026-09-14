package rri

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

// TestSubmitMonthlyReportRequestShape pins the wire format for both reports:
// the two path spellings, the absence of an /info/ prefix, the CSV content
// type mandated by Section 3, and a body that reaches the server byte for byte.
func TestSubmitMonthlyReportRequestShape(t *testing.T) {
	tests := []struct {
		name    string
		typ     ReportType
		fixture string
		path    string
	}{
		{
			name:    "transactions",
			typ:     ReportTransactions,
			fixture: "example-transactions-202501.csv",
			path:    "/report/registrar-transactions/example/2025-01",
		},
		{
			name: "activity",
			typ:  ReportActivity,
			// BOM included on purpose: it must survive to the wire.
			fixture: "example-activity-202501-bom.csv",
			path:    "/report/registry-functions-activity/example/2025-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := readFixture(t, tt.fixture)

			var (
				gotMethod, gotPath, gotContentType string
				gotHasAuth                         bool
				gotLength                          int64
				gotBody                            []byte
			)
			cli := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotContentType = r.Header.Get("Content-Type")
				_, _, gotHasAuth = r.BasicAuth()
				gotLength = r.ContentLength
				gotBody, _ = readAll(r)
				respond(w, http.StatusOK, ResultSuccess)
			})

			res, err := cli.SubmitMonthlyReport(context.Background(), tt.typ, "2025-01", fixture)
			if err != nil {
				t.Fatalf("SubmitMonthlyReport() error = %v", err)
			}

			if gotMethod != http.MethodPut {
				t.Errorf("method = %q, want PUT", gotMethod)
			}
			if gotPath != tt.path {
				t.Errorf("path = %q, want %q", gotPath, tt.path)
			}
			if strings.Contains(gotPath, "/info/") {
				t.Error("path contains /info/, which belongs to the status endpoint only")
			}
			if gotContentType != "text/csv" {
				t.Errorf("Content-Type = %q, want text/csv", gotContentType)
			}
			if !gotHasAuth {
				t.Error("request carried no Authorization header")
			}
			if gotLength != int64(len(fixture)) {
				t.Errorf("ContentLength = %d, want %d", gotLength, len(fixture))
			}
			if !bytes.Equal(gotBody, fixture) {
				t.Error("body was not transmitted verbatim; the report must never be re-serialized")
			}
			if res.Type != tt.typ || res.Month != "2025-01" || res.ResultCode != ResultSuccess {
				t.Errorf("result = %+v", res)
			}
		})
	}
}

func TestSubmitMonthlyReport(t *testing.T) {
	body := readFixture(t, "example-activity-202501.csv")

	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		wantCode   int  // expected *ResultError code, 0 if none
		wantHTTP   bool // expect a *client.HTTPError
		wantRetry  bool
		wantAccept bool
	}{
		{
			name:       "accepted",
			handler:    func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, ResultSuccess) },
			wantAccept: true,
		},
		{
			// The single most important case: ICANN rejects with HTTP 200.
			name:     "incorrect totals rejected with http 200",
			handler:  func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, ResultIncorrectTotals) },
			wantCode: ResultIncorrectTotals,
		},
		{
			name: "cut-off date passed is not retryable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				respond(w, http.StatusBadRequest, ResultReportExistsCutOff)
			},
			wantCode: ResultReportExistsCutOff,
		},
		{
			name:      "interface disabled is retryable",
			handler:   func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, ResultInterfaceDisabled) },
			wantCode:  ResultInterfaceDisabled,
			wantRetry: true,
		},
		{
			name:     "not utf-8",
			handler:  func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusBadRequest, ResultNotUTF8) },
			wantCode: ResultNotUTF8,
		},
		{
			name:     "unauthorized",
			handler:  func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			wantHTTP: true,
		},
		{
			name: "server failure is retryable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "<html>oops</html>", http.StatusInternalServerError)
			},
			wantHTTP:  true,
			wantRetry: true,
		},
		{
			name: "unparseable body is never a silent success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not xml at all"))
			},
			wantHTTP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := newTestRRI(t, tt.handler)
			res, err := cli.SubmitMonthlyReport(context.Background(), ReportActivity, "2025-01", body)

			if tt.wantAccept {
				if err != nil {
					t.Fatalf("SubmitMonthlyReport() error = %v, want success", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("SubmitMonthlyReport() succeeded, want an error; got %+v", res)
			}
			if res != nil {
				t.Errorf("result = %+v on error, want nil", res)
			}
			if got := IsRetryable(err); got != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.wantRetry)
			}

			var re *ResultError
			var he *base.HTTPError
			isResult := errors.As(err, &re)
			isHTTP := errors.As(err, &he)

			if tt.wantCode != 0 {
				if !isResult {
					t.Fatalf("error %v is not a *ResultError", err)
				}
				if re.Code != tt.wantCode {
					t.Errorf("result code = %d, want %d", re.Code, tt.wantCode)
				}
				// The two error kinds are alternatives, never nested.
				if isHTTP {
					t.Error("a rejection must not also be a *client.HTTPError")
				}
			}
			if tt.wantHTTP {
				if !isHTTP {
					t.Fatalf("error %v is not a *client.HTTPError", err)
				}
				if isResult {
					t.Error("a transport failure must not also be a *ResultError")
				}
			}
		})
	}
}

// TestSubmitMonthlyReportRejectsBadInput checks the guards fire before any
// request is issued, so a typo never costs an authenticated round trip.
func TestSubmitMonthlyReportRejectsBadInput(t *testing.T) {
	var requests int
	cli := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		respond(w, http.StatusOK, ResultSuccess)
	})
	body := readFixture(t, "example-activity-202501.csv")

	tests := []struct {
		name  string
		typ   ReportType
		month string
		body  []byte
	}{
		{"unknown type", ReportType("escrow"), "2025-01", body},
		{"empty type", ReportType(""), "2025-01", body},
		{"month with a day", ReportActivity, "2025-01-01", body},
		{"month 13", ReportActivity, "2025-13", body},
		{"month 00", ReportActivity, "2025-00", body},
		{"empty month", ReportActivity, "", body},
		{"empty body", ReportActivity, "2025-01", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := cli.SubmitMonthlyReport(context.Background(), tt.typ, tt.month, tt.body); err == nil {
				t.Fatal("SubmitMonthlyReport() succeeded, want an error")
			}
		})
	}
	if requests != 0 {
		t.Errorf("server saw %d requests, want 0: every guard must fire locally", requests)
	}
}

// TestSubmitMonthlyReportReusesConnection guards the rate-limit requirement.
// ICANN rate-limits on authentication and there is no session to carry, so a
// batch must run over one connection. A client built per file, or a response
// body left undrained, fails here rather than in production.
func TestSubmitMonthlyReportReusesConnection(t *testing.T) {
	var requests int
	cli := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		respond(w, http.StatusOK, ResultSuccess)
	})

	var reused []bool
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = append(reused, info.Reused) },
	})

	// Mixed types on purpose: both must share the one connection.
	submissions := []struct {
		typ   ReportType
		file  string
		month string
	}{
		{ReportTransactions, "example-transactions-202501.csv", "2025-01"},
		{ReportActivity, "example-activity-202501.csv", "2025-01"},
		{ReportTransactions, "example-transactions-202501.csv", "2025-02"},
		{ReportActivity, "example-activity-202501.csv", "2025-02"},
	}
	for _, s := range submissions {
		if _, err := cli.SubmitMonthlyReport(ctx, s.typ, s.month, readFixture(t, s.file)); err != nil {
			t.Fatalf("SubmitMonthlyReport(%s, %s): %v", s.typ, s.month, err)
		}
	}

	if requests != len(submissions) {
		t.Errorf("server saw %d requests, want %d", requests, len(submissions))
	}
	want := []bool{false, true, true, true}
	if len(reused) != len(want) {
		t.Fatalf("opened %d connections for %d reports, want 1", len(reused), len(submissions))
	}
	for i, w := range want {
		if reused[i] != w {
			t.Errorf("submission %d: connection reused = %v, want %v", i+1, reused[i], w)
		}
	}
}

func TestGetMonthlyReportStatus(t *testing.T) {
	tests := []struct {
		name     string
		typ      ReportType
		status   int
		wantPath string
		want     string
		wantErr  bool
	}{
		{"received", ReportTransactions, http.StatusOK, "/info/report/registrar-transactions/example/2025-01", RY_RDEReport_RECEIVED, false},
		{"pending", ReportActivity, http.StatusNotFound, "/info/report/registry-functions-activity/example/2025-01", RY_RDEReport_PENDING, false},
		{"server failure", ReportActivity, http.StatusInternalServerError, "/info/report/registry-functions-activity/example/2025-01", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotMethod string
			cli := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				w.WriteHeader(tt.status)
			})
			st, err := cli.GetMonthlyReportStatus(context.Background(), tt.typ, "2025-01")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetMonthlyReportStatus() succeeded, want an error; got %+v", st)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetMonthlyReportStatus() error = %v", err)
			}
			if gotMethod != http.MethodHead {
				t.Errorf("method = %q, want HEAD", gotMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if st.Status != tt.want {
				t.Errorf("status = %q, want %q", st.Status, tt.want)
			}
			if st.Date.Format("2006-01") != "2025-01" {
				t.Errorf("date = %s, want the first of 2025-01", st.Date)
			}
		})
	}
}

func TestResultHint(t *testing.T) {
	// The cut-off code is the one a backfill is most likely to hit, and a bare
	// number is not actionable.
	if h := ResultHint(ResultReportExistsCutOff); !strings.Contains(h, "cut-off") {
		t.Errorf("ResultHint(%d) = %q, want it to explain the cut-off date", ResultReportExistsCutOff, h)
	}
	if h := ResultHint(ResultSuccess); h != "" {
		t.Errorf("ResultHint(1000) = %q, want no hint for success", h)
	}
	if h := ResultHint(9999); h != "" {
		t.Errorf("ResultHint(9999) = %q, want no hint for an unknown code", h)
	}
}
