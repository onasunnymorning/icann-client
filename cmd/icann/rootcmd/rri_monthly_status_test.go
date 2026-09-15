package rootcmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

// runMonthlyStatusCmd drives `icann get monthly status` against a test server
// and returns the decoded stdout, the request paths the server saw, and the
// command's error.
func runMonthlyStatusCmd(t *testing.T, args []string, h http.HandlerFunc) (rri.ReportStatus, []string, error) {
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
	cmd := rriMonthlyStatusCmd
	cmd.SetOut(&stdout)
	cmd.SetContext(t.Context())
	t.Cleanup(func() { cmd.SetOut(nil) })

	full := append([]string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p"}, args...)
	if err := cmd.ParseFlags(full); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	runErr := cmd.RunE(cmd, cmd.Flags().Args())

	var status rri.ReportStatus
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
			t.Fatalf("decoding stdout %q: %v", stdout.String(), err)
		}
	}
	return status, paths, runErr
}

// TestMonthlyStatusRequestShape pins the path, because copying it from the
// submit command would drop the /info/ segment and silently probe the
// submission endpoint instead.
func TestMonthlyStatusRequestShape(t *testing.T) {
	for _, tc := range []struct {
		typ  string
		want string
	}{
		{"transactions", "HEAD /info/report/registrar-transactions/example/2025-01"},
		{"activity", "HEAD /info/report/registry-functions-activity/example/2025-01"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			_, paths, err := runMonthlyStatusCmd(t, []string{"--type", tc.typ, "--month", "2025-01"},
				func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
			if err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if len(paths) != 1 || paths[0] != tc.want {
				t.Errorf("requests = %v, want exactly [%s]", paths, tc.want)
			}
		})
	}
}

// TestMonthlyStatusTypeIsRequired covers the two ways --type can be wrong.
// Both must fail before a request is issued, since a status probe against the
// wrong report type reads as a genuine "not received".
func TestMonthlyStatusTypeIsRequired(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		// The error must name the flag, not just the type. Without the
		// command's own guard the request still fails, but down in the rri
		// package with a message about an "unknown report type" that never
		// tells the operator which flag to set.
		wants []string
	}{
		"missing": {[]string{"--month", "2025-01"}, []string{"--type", "transactions", "activity"}},
		"bogus":   {[]string{"--type", "quarterly", "--month", "2025-01"}, []string{"--type", "transactions", "activity"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, paths, err := runMonthlyStatusCmd(t, tc.args,
				func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
			if err == nil {
				t.Fatal("RunE() error = nil, want an error naming the valid types")
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if len(paths) != 0 {
				t.Errorf("requests = %v, want none before the flag is validated", paths)
			}
		})
	}
}

// TestMonthlyStatusRejectsBadMonth pre-empts result code 2111 without spending
// a request.
func TestMonthlyStatusRejectsBadMonth(t *testing.T) {
	_, paths, err := runMonthlyStatusCmd(t, []string{"--type", "activity", "--month", "2025-1"},
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM") {
		t.Fatalf("RunE() error = %v, want an error naming the YYYY-MM form", err)
	}
	if len(paths) != 0 {
		t.Errorf("requests = %v, want none", paths)
	}
}

// TestMonthlyStatusDefaultsToPreviousMonth guards the default: the current
// month is never accepted by ICANN, so defaulting to it would make the
// command useless without --month.
func TestMonthlyStatusDefaultsToPreviousMonth(t *testing.T) {
	want := "/info/report/registry-functions-activity/example/" + previousMonth(time.Now())
	_, paths, err := runMonthlyStatusCmd(t, []string{"--type", "activity"},
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	if err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], want) {
		t.Errorf("requests = %v, want one ending in %s", paths, want)
	}
}

// TestPreviousMonth pins the arithmetic across a year boundary and from a day
// that does not exist in the previous month.
func TestPreviousMonth(t *testing.T) {
	for _, tc := range []struct{ from, want string }{
		{"2026-01-01", "2025-12"},
		{"2026-03-31", "2026-02"},
		{"2026-09-14", "2026-08"},
	} {
		now, err := time.Parse("2006-01-02", tc.from)
		if err != nil {
			t.Fatalf("time.Parse(%q): %v", tc.from, err)
		}
		if got := previousMonth(now); got != tc.want {
			t.Errorf("previousMonth(%s) = %q, want %q", tc.from, got, tc.want)
		}
	}
}

// TestMonthlyStatusReportsStatus checks both branches reach stdout as a status
// rather than, in the 404 case, as an error.
func TestMonthlyStatusReportsStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		want string
	}{
		{"received", http.StatusOK, rri.RY_RDEReport_RECEIVED},
		{"pending", http.StatusNotFound, rri.RY_RDEReport_PENDING},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, err := runMonthlyStatusCmd(t, []string{"--type", "transactions", "--month", "2025-01"},
				func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code) })
			if err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if status.Status != tc.want {
				t.Errorf("status = %q, want %q", status.Status, tc.want)
			}
			if status.TLD != "example" || status.Type != string(rri.ReportTransactions) {
				t.Errorf("status = %+v, want tld example and type transactions", status)
			}
		})
	}
}
