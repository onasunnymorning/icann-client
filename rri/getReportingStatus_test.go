package rri

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

// TestGetReportingStatusRequestShape pins the path. Copying it from the
// submission files is the likeliest way to get this wrong, and a stray
// /report/ segment would probe a completely different resource.
func TestGetReportingStatusRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotAccept string
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAccept = r.Method, r.URL.Path, r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/xml")
		w.Write(readFixture(t, "reporting-summary-issues.xml"))
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
	// ICANN answered 406 Not Acceptable to Accept: text/xml here, and the
	// draft requires no Accept header at all, so sending one is a regression.
	if gotAccept != "" {
		t.Errorf("Accept = %q, want no Accept header: ICANN rejects a constrained one with 406", gotAccept)
	}
}

// TestGetReportingStatusDecodes is the case that matters for a backfill: the
// issue list is what says which dates ICANN is still missing.
func TestGetReportingStatusDecodes(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write(readFixture(t, "reporting-summary-issues.xml"))
	})

	got, err := c.GetReportingStatus(t.Context())
	if err != nil {
		t.Fatalf("GetReportingStatus() error = %v", err)
	}

	want := &ReportingSummary{
		TLD:             "example",
		CreationDate:    "2026-01-02T12:00:30.101Z",
		DepositSchedule: DepositScheduleDaily,
		LastFullDate:    "2026-01-01",
		Timestamp:       "2026-01-02T12:00:00.000Z",
		Reports: []ReportTypeStatus{
			{
				Type:    ReportingDEANotification,
				Enabled: true,
				Status:  ReportingStatusUnsatisfactory,
				Issues: []ReportingIssue{
					{Date: "2026-01-01", Description: IssueNoReportReceived},
					{Date: "2025-12-30", Description: IssueInvalidDepositFull},
				},
			},
			{Type: ReportingActivityReport, Enabled: true, Status: ReportingStatusOK},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetReportingStatus() =\n%+v\nwant\n%+v", got, want)
	}
}

// TestGetReportingStatusNamespaceIndependent guards the decode-only structs
// against a server that declares the namespace as the default instead of using
// a prefix. Both spellings are the same document, so both must decode
// identically.
//
// Note what this does and does not prove. encoding/xml matches an unqualified
// tag in any namespace, so it will not catch a tag that simply omits its
// namespace. What it does catch is a tag given the wrong one — most usefully
// tld, which lives in the rdeHeader namespace rather than the rriReporting one
// that surrounds it, and which decodes to the empty string if that is
// "tidied up".
func TestGetReportingStatusNamespaceIndependent(t *testing.T) {
	get := func(fixture string) *ReportingSummary {
		c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write(readFixture(t, fixture))
		})
		got, err := c.GetReportingStatus(t.Context())
		if err != nil {
			t.Fatalf("GetReportingStatus(%s) error = %v", fixture, err)
		}
		if len(got.Reports) == 0 {
			t.Fatalf("GetReportingStatus(%s) decoded no status reports", fixture)
		}
		return got
	}
	prefixed := get("reporting-summary-issues.xml")
	defaultNS := get("reporting-summary-default-ns.xml")
	if !reflect.DeepEqual(prefixed, defaultNS) {
		t.Errorf("prefixed =\n%+v\ndefault namespace =\n%+v", prefixed, defaultNS)
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

// TestGetReportingStatusTransportErrors checks that nothing decodes into an
// empty-but-successful summary.
func TestGetReportingStatusTransportErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		"401 with no body": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"200 with a login page": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body>Please sign in</body></html>"))
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
	s := &ReportingSummary{Reports: []ReportTypeStatus{
		{Type: ReportingEscrowReport, Status: ReportingStatusOK},
		{Type: ReportingActivityReport, Status: ReportingStatusUnsatisfactory},
	}}
	got := s.Unsatisfactory()
	if len(got) != 1 || got[0].Type != ReportingActivityReport {
		t.Errorf("Unsatisfactory() = %+v, want only the activity report", got)
	}
	clean := &ReportingSummary{Reports: []ReportTypeStatus{{Status: ReportingStatusOK}}}
	if len(clean.Unsatisfactory()) != 0 {
		t.Errorf("Unsatisfactory() on a clean summary = %+v, want none", clean.Unsatisfactory())
	}
}

// TestNamespaceConstantsMatchTags guards against drift. Struct tags must hold
// the namespace as a literal, so the exported constants are a second copy of
// the same URIs; this keeps the two from disagreeing.
func TestNamespaceConstantsMatchTags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		field    reflect.StructTag
		constant string
	}{
		{"rriReporting", reflect.TypeOf(xmlReportingSummary{}).Field(0).Tag, NamespaceRRIReporting},
		{"rdeHeader", reflect.TypeOf(xmlReportingSummary{}).Field(1).Tag, NamespaceRdeHeader},
		{"rriConformance", reflect.TypeOf(xmlConformance{}).Field(0).Tag, NamespaceRRIConformance},
	} {
		got, _ := tc.field.Lookup("xml")
		if !strings.HasPrefix(got, tc.constant+" ") {
			t.Errorf("%s: tag %q does not use the namespace constant %q", tc.name, got, tc.constant)
		}
	}
}
