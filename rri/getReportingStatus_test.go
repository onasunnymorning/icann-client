package rri

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

// TestGetReportingStatusRequestShape pins the path and the absence of an
// Accept header. ICANN answers 406 Not Acceptable to "Accept: text/xml" here,
// because it cannot produce XML for this resource, so sending one is a
// regression rather than a preference.
func TestGetReportingStatusRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotAccept string
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAccept = r.Method, r.URL.Path, r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write(readFixture(t, "reporting-summary.json"))
	})

	if _, err := c.GetReportingStatus(t.Context()); err != nil {
		t.Fatalf("GetReportingStatus() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if want := "/info/status/registry/example"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotAccept != "" {
		t.Errorf("Accept = %q, want none: ICANN answers 406 to a constrained one", gotAccept)
	}
}

// TestGetReportingStatusDecodes uses a capture of a real production response.
// ICANN serves JSON here, not the XML document the draft describes, and nests
// the TLD as {"tld": {"name": ...}}.
func TestGetReportingStatusDecodes(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(readFixture(t, "reporting-summary.json"))
	})

	got, err := c.GetReportingStatus(t.Context())
	if err != nil {
		t.Fatalf("GetReportingStatus() error = %v", err)
	}

	want := &ReportingSummary{
		TLD:     "example",
		Created: "2026-09-15T00:44:03.230Z",
		Paths: []ReportingPath{
			{Path: ReportingPathFull, Status: ReportingStatusOK},
			{Path: ReportingPathDiff, Status: ReportingStatusOK},
			{Path: ReportingPathDea, Status: ReportingStatusOK},
			{Path: ReportingPathPRTR, Status: "unsatisfactory"},
			{Path: ReportingPathRFAR, Status: ReportingStatusOK},
			{Path: ReportingPathRegistry, Status: ReportingStatusOK},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetReportingStatus() =\n%+v\nwant\n%+v", got, want)
	}
}

// TestGetReportingStatusDecodeFailure guards the error a malformed body
// produces. Reporting it as an *client.HTTPError produced "http error: 200",
// which describes a successful request and sends the reader looking in the
// wrong place; the cause is the payload.
func TestGetReportingStatusDecodeFailure(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tld": "not the expected shape"}`))
	})

	got, err := c.GetReportingStatus(t.Context())
	if err == nil {
		t.Fatalf("GetReportingStatus() error = nil, got %+v", got)
	}
	var he *base.HTTPError
	if errors.As(err, &he) {
		t.Errorf("error is a *client.HTTPError (%v); a 200 whose body will not decode is not an HTTP failure", err)
	}
	for _, want := range []string{"HTTP 200", "reporting summary", "not the expected shape"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestGetReportingStatusRejection covers ICANN answering a read with the same
// result envelope it uses for a rejected submission, including on HTTP 200.
func TestGetReportingStatusRejection(t *testing.T) {
	const envelope = `<?xml version="1.0" encoding="UTF-8"?>
<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0">
  <result code="2007"><msg>Interface disabled for this TLD</msg></result>
</response>`

	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(status)
			w.Write([]byte(envelope))
		})
		_, err := c.GetReportingStatus(t.Context())
		var re *ResultError
		if !errors.As(err, &re) {
			t.Fatalf("http %d: error = %v (%T), want *ResultError", status, err, err)
		}
		if re.Code != ResultInterfaceDisabled {
			t.Errorf("http %d: code = %d, want %d", status, re.Code, ResultInterfaceDisabled)
		}
		if !IsRetryable(err) {
			t.Errorf("http %d: IsRetryable() = false, want true for a disabled interface", status)
		}
	}
}

func TestGetReportingStatusTransportErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		"401 with no body": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"406 as production answered a constrained Accept": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotAcceptable)
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestRRI(t, h)
			got, err := c.GetReportingStatus(t.Context())
			var he *base.HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("error = %v (%T), want *client.HTTPError, got summary %+v", err, err, got)
			}
			if got != nil {
				t.Errorf("summary = %+v, want nil alongside an error", got)
			}
		})
	}
}

// TestUnsatisfactory is what `get reporting status --issues-only` filters on.
func TestUnsatisfactory(t *testing.T) {
	s := &ReportingSummary{Paths: []ReportingPath{
		{Path: ReportingPathFull, Status: ReportingStatusOK},
		{Path: ReportingPathRFAR, Status: "unsatisfactory"},
	}}
	got := s.Unsatisfactory()
	if len(got) != 1 || got[0].Path != ReportingPathRFAR {
		t.Errorf("Unsatisfactory() = %+v, want only RFAR", got)
	}
	clean := &ReportingSummary{Paths: []ReportingPath{{Status: ReportingStatusOK}}}
	if len(clean.Unsatisfactory()) != 0 {
		t.Errorf("Unsatisfactory() on a clean summary = %+v, want none", clean.Unsatisfactory())
	}
}
