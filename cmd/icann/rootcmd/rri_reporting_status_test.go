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

// summaryFixture is a capture of a real production response, with the TLD
// changed. ICANN serves JSON here, not the XML the draft describes.
const summaryFixture = `{
  "tld" : { "name" : "example" },
  "paths" : [
    { "path" : "Full", "status" : "ok" },
    { "path" : "PRTR", "status" : "unsatisfactory" },
    { "path" : "RFAR", "status" : "ok" }
  ],
  "created" : "2026-09-15T00:44:03.230Z"
}`

func TestReportingStatusCmd(t *testing.T) {
	out, paths, err := runGetCmd(t, rriReportingStatusCmd, nil, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
	if len(got.Paths) != 3 {
		t.Errorf("paths = %d, want all three from the fixture", len(got.Paths))
	}
	if got.TLD != "example" {
		t.Errorf("tld = %q, want example: ICANN nests it as {\"tld\": {\"name\": ...}}", got.TLD)
	}
	if got.Created == "" {
		t.Error("created is empty; it is the only timestamp in the answer")
	}
}

// TestReportingStatusIssuesOnly is the form a script uses: the unsatisfactory
// obligations on stdout and a non-zero exit, so a check fails loudly.
func TestReportingStatusIssuesOnly(t *testing.T) {
	out, _, err := runGetCmd(t, rriReportingStatusCmd, []string{"--issues-only"}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
	if len(got.Paths) != 1 || got.Paths[0].Path != rri.ReportingPathPRTR {
		t.Errorf("paths = %+v, want only the unsatisfactory one", got.Paths)
	}
}

// TestReportingStatusIssuesOnlyClean is the other half: a TLD that is square
// with ICANN must exit zero, or the check is useless. This is what .radio
// actually returns.
func TestReportingStatusIssuesOnlyClean(t *testing.T) {
	const clean = `{
  "tld" : { "name" : "example" },
  "paths" : [
    { "path" : "Full", "status" : "ok" },
    { "path" : "RFAR", "status" : "ok" }
  ],
  "created" : "2026-09-15T00:44:03.230Z"
}`

	out, _, err := runGetCmd(t, rriReportingStatusCmd, []string{"--issues-only"}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(clean))
	})
	if err != nil {
		t.Fatalf("RunE() error = %v, want a zero exit for a TLD with no issues", err)
	}
	var got rri.ReportingSummary
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	if len(got.Paths) != 0 {
		t.Errorf("paths = %+v, want none", got.Paths)
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
