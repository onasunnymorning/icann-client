package rootcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/mosapi"
	"github.com/onasunnymorning/icann-client/rri"
)

// mosapiStateStub starts a MOSAPI stand-in that accepts the session login
// MOSAPI requires for basic auth, then answers /monitoring/state with state.
func mosapiStateStub(t *testing.T, state http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ry/example/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "id", Value: "sess", Path: "/ry/example"})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "Login successful")
	})
	mux.HandleFunc("/ry/example/v2/monitoring/state", state)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const reportingSummaryAllOK = `{
  "tld": {"name": "example"},
  "created": "2026-09-15T00:44:03.230Z",
  "paths": [
    {"path": "Full", "status": "ok"},
    {"path": "Diff", "status": "ok"},
    {"path": "Dea", "status": "ok"},
    {"path": "PRTR", "status": "ok"},
    {"path": "RFAR", "status": "ok"},
    {"path": "Registry", "status": "ok"}
  ]
}`

const reportingSummaryWithIssue = `{
  "tld": {"name": "example"},
  "created": "2026-09-15T00:44:03.230Z",
  "paths": [
    {"path": "Full", "status": "ok"},
    {"path": "Diff", "status": "ok"},
    {"path": "Dea", "status": "ok"},
    {"path": "PRTR", "status": "unsatisfactory"},
    {"path": "RFAR", "status": "ok"},
    {"path": "Registry", "status": "ok"}
  ]
}`

// runStatusCmd drives `icann status` against stub MOSAPI/RRI servers and
// returns stdout and the command's error.
func runStatusCmd(t *testing.T, args []string, mosSrv, rriSrv *httptest.Server) (string, error) {
	t.Helper()

	prevRRI, prevMos := newRRIClient, newMosapiClient
	newRRIClient = func(cfg base.Config) (*rri.Client, error) {
		c, err := rri.New(cfg)
		if err != nil {
			return nil, err
		}
		return c, c.WithBaseURL(rriSrv.URL)
	}
	newMosapiClient = func(cfg base.Config) (*mosapi.Client, error) {
		c, err := mosapi.New(cfg)
		if err != nil {
			return nil, err
		}
		return c, c.WithBaseURL(mosSrv.URL)
	}
	t.Cleanup(func() { newRRIClient, newMosapiClient = prevRRI, prevMos })
	t.Cleanup(resetSubmitFlags)
	t.Cleanup(func() { flagStatusJSON = false })
	resetSubmitFlags()
	flagStatusJSON = false

	var stdout bytes.Buffer
	cmd := statusCmd
	cmd.SetOut(&stdout)
	cmd.SetContext(t.Context())
	t.Cleanup(func() { cmd.SetOut(nil) })

	full := append([]string{"--tld", "example", "--auth", "basic", "--username", "u", "--password", "p"}, args...)
	if err := cmd.ParseFlags(full); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	runErr := cmd.RunE(cmd, cmd.Flags().Args())
	return stdout.String(), runErr
}

func jsonState(status string, services map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tested := map[string]mosapi.TestedService{}
		for name, st := range services {
			tested[name] = mosapi.TestedService{Status: st}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mosapi.StateResponse{
			TLD: "example", Status: status, TestedServices: tested, Version: 1,
		})
	}
}

// TestStatusHumanSummaryHappyPath checks the default, human-readable output
// when SLA monitoring is up and every reporting obligation is satisfied.
func TestStatusHumanSummaryHappyPath(t *testing.T) {
	mosSrv := mosapiStateStub(t, jsonState("Up", map[string]string{"DNS": "Up"}))
	rriSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reportingSummaryAllOK)
	}))
	t.Cleanup(rriSrv.Close)

	out, err := runStatusCmd(t, nil, mosSrv, rriSrv)
	if err != nil {
		t.Fatalf("RunE() error = %v, want nil", err)
	}
	for _, want := range []string{"TLD: example", "SLA monitoring:", "Up", "DNS:", "[ok] Full registry escrow deposit"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}
}

// TestStatusReportsUnsatisfactoryObligation checks that an unsatisfactory
// reporting obligation both shows up in the summary and makes the command
// exit non-zero, so it can be used as a monitoring check.
func TestStatusReportsUnsatisfactoryObligation(t *testing.T) {
	mosSrv := mosapiStateStub(t, jsonState("Up", map[string]string{"DNS": "Up"}))
	rriSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reportingSummaryWithIssue)
	}))
	t.Cleanup(rriSrv.Close)

	out, err := runStatusCmd(t, nil, mosSrv, rriSrv)
	if err == nil {
		t.Fatal("RunE() error = nil, want an error naming the unsatisfactory obligation")
	}
	if !strings.Contains(err.Error(), "1 reporting obligation") {
		t.Errorf("error = %q, want it to count the unsatisfactory obligation", err)
	}
	if !strings.Contains(out, "[UNSATISFACTORY] Monthly per-registrar transactions report") || !strings.Contains(out, "(status: unsatisfactory)") {
		t.Errorf("output = %q, want the obligation flagged UNSATISFACTORY with its status", out)
	}
}

// TestStatusSLADown checks that a "Down" SLA status is surfaced and makes the
// command exit non-zero.
func TestStatusSLADown(t *testing.T) {
	mosSrv := mosapiStateStub(t, jsonState("Down", map[string]string{"DNS": "Down"}))
	rriSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reportingSummaryAllOK)
	}))
	t.Cleanup(rriSrv.Close)

	_, err := runStatusCmd(t, nil, mosSrv, rriSrv)
	if err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("RunE() error = %v, want an error naming the TLD down", err)
	}
}

// TestStatusPartialFailure checks that a failure in one check (here, RRI) does
// not prevent the other (MOSAPI) from being reported, and that the failure is
// named rather than aborting the whole command.
func TestStatusPartialFailure(t *testing.T) {
	mosSrv := mosapiStateStub(t, jsonState("Up", map[string]string{"DNS": "Up"}))
	rriSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(rriSrv.Close)

	out, err := runStatusCmd(t, nil, mosSrv, rriSrv)
	if err == nil || !strings.Contains(err.Error(), "reporting status:") {
		t.Fatalf("RunE() error = %v, want it to name the reporting-status failure", err)
	}
	if !strings.Contains(out, "SLA monitoring:\n  Up") {
		t.Errorf("output = %q, want the SLA section to still report Up despite the other failure", out)
	}
	if !strings.Contains(out, "Reporting obligations") || !strings.Contains(out, "could not check:") {
		t.Errorf("output = %q, want the reporting section to say it could not be checked", out)
	}
}

// TestStatusJSON checks that --json prints the structured form instead of the
// human summary, and that it round-trips into statusReport.
func TestStatusJSON(t *testing.T) {
	mosSrv := mosapiStateStub(t, jsonState("Up", map[string]string{"DNS": "Up"}))
	rriSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reportingSummaryAllOK)
	}))
	t.Cleanup(rriSrv.Close)

	out, err := runStatusCmd(t, []string{"--json"}, mosSrv, rriSrv)
	if err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	var got statusReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decoding stdout %q: %v", out, err)
	}
	if got.TLD != "example" {
		t.Errorf("TLD = %q, want example", got.TLD)
	}
	if got.SLA == nil || got.SLA.Status != "Up" {
		t.Errorf("SLA = %+v, want status Up", got.SLA)
	}
	if got.Reporting == nil || len(got.Reporting.Paths) != 6 || got.Reporting.Paths[0].Path != rri.ReportingPathFull {
		t.Errorf("Reporting = %+v, want six paths starting with Full", got.Reporting)
	}
}
