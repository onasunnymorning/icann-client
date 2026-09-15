package rri

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// NamespaceRRIReporting is the XML namespace of the reporting status document.
const NamespaceRRIReporting = "urn:ietf:params:xml:ns:rriReporting-1.0"

// The reporting obligations ICANN tracks per TLD.
const (
	ReportingDEANotification    = "DEA_Notification"                           // the escrow agent's notification
	ReportingEscrowReport       = "Registry_Escrow_Report"                     // Specification 2
	ReportingTransactionsReport = "Registry_Per_Registrar_Transactions_Report" // Specification 3 Section 1
	ReportingActivityReport     = "Registry_Functions_Activity_Report"         // Specification 3 Section 2
)

// Values of ReportTypeStatus.Status.
const (
	ReportingStatusOK             = "ok"
	ReportingStatusUnsatisfactory = "unsatisfactory"
)

// Values of ReportingIssue.Description.
const (
	IssueMissingDepositFull = "Missing_Deposit_Full" // a full deposit was never received
	IssueMissingDepositDiff = "Missing_Deposit_Diff" // a differential deposit was never received
	IssueInvalidDepositFull = "Invalid_Deposit_Full" // a full deposit arrived but did not validate
	IssueInvalidDepositDiff = "Invalid_Deposit_Diff" // a differential deposit arrived but did not validate
	IssueNoReportReceived   = "No_Report_Received"   // no report at all arrived for that date
)

// Values of ReportingSummary.DepositSchedule.
const (
	DepositScheduleNone   = "None"
	DepositScheduleWeekly = "Weekly"
	DepositScheduleDaily  = "Daily"
)

// ReportingIssue is one dated problem ICANN has recorded against a reporting
// obligation.
type ReportingIssue struct {
	Date        string `json:"date"`        // YYYY-MM-DD
	Description string `json:"description"` // one of the Issue* constants
}

// ReportTypeStatus is ICANN's view of one reporting obligation for the TLD.
type ReportTypeStatus struct {
	Type    string           `json:"type"` // one of the Reporting* type constants
	Enabled bool             `json:"enabled"`
	Status  string           `json:"status"` // ok or unsatisfactory
	Issues  []ReportingIssue `json:"issues,omitempty"`
}

// ReportingSummary is the per-TLD reporting status ICANN maintains: which
// obligations are enabled, whether each is satisfied, and every dated issue
// recorded against it.
//
// Dates are kept as strings because ICANN sends two different shapes —
// CreationDate and Timestamp are full timestamps, LastFullDate and an issue's
// Date are plain YYYY-MM-DD — and every consumer either prints them or
// compares them lexically, which both shapes support.
type ReportingSummary struct {
	TLD             string             `json:"tld"`
	CreationDate    string             `json:"creationDate,omitempty"`
	DepositSchedule string             `json:"depositSchedule,omitempty"` // None, Weekly or Daily
	LastFullDate    string             `json:"lastFullDate,omitempty"`
	Timestamp       string             `json:"timestamp,omitempty"`
	Reports         []ReportTypeStatus `json:"reports"`
}

// Unsatisfactory returns the reporting obligations ICANN is not satisfied
// with. An empty result means the TLD is square with ICANN.
func (s *ReportingSummary) Unsatisfactory() []ReportTypeStatus {
	var out []ReportTypeStatus
	for _, r := range s.Reports {
		if r.Status != ReportingStatusOK {
			out = append(out, r)
		}
	}
	return out
}

// xmlReportingSummary and friends are decode-only mirrors of the response.
// The tags are namespace-qualified so a document using a different prefix, or
// declaring a namespace as the default, decodes identically. Note that tld
// comes from the rdeHeader namespace, not the rriReporting one.
type xmlReportingSummary struct {
	XMLName         xml.Name         `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 summary"`
	TLD             string           `xml:"urn:ietf:params:xml:ns:rdeHeader-1.0 tld"`
	CreationDate    string           `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 creationDate"`
	DepositSchedule string           `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 depositSchedule"`
	LastFullDate    string           `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 lastFullDate"`
	Timestamp       string           `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 timestamp"`
	StatusReports   xmlStatusReports `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 statusReports"`
}

// The wrapper elements are modelled as their own structs rather than with the
// encoding/xml "parent>child" path syntax, which silently ignores a namespace
// on the path segments and decodes nothing at all.
type xmlStatusReports struct {
	Reports []xmlStatusReport `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 statusReport"`
}

type xmlStatusReport struct {
	Type    string    `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 type"`
	Enabled bool      `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 enabled"`
	Status  string    `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 status"`
	Issues  xmlIssues `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 issues"`
}

type xmlIssues struct {
	Issues []xmlIssue `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 issue"`
}

type xmlIssue struct {
	Date        string `xml:"date,attr"`
	Description string `xml:"description,attr"`
}

// GetReportingStatus returns ICANN's own view of the client's TLD reporting:
// which obligations are enabled, whether each is satisfied, and every dated
// issue recorded against it, per
// draft-lozano-icann-registry-interfaces Section 6.
//
// This is the endpoint that answers "which reports does ICANN think are
// missing?" without submitting anything.
func (c *Client) GetReportingStatus(ctx context.Context) (*ReportingSummary, error) {
	cfg := c.Config()
	path := fmt.Sprintf("/info/status/registry/%s", url.PathEscape(cfg.TLD))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/xml")

	var doc xmlReportingSummary
	if _, err := c.doXMLGet(req, &doc); err != nil {
		return nil, err
	}

	out := &ReportingSummary{
		TLD:             doc.TLD,
		CreationDate:    doc.CreationDate,
		DepositSchedule: doc.DepositSchedule,
		LastFullDate:    doc.LastFullDate,
		Timestamp:       doc.Timestamp,
	}
	for _, r := range doc.StatusReports.Reports {
		st := ReportTypeStatus{Type: r.Type, Enabled: r.Enabled, Status: r.Status}
		for _, i := range r.Issues.Issues {
			// xmlIssue exists only to carry the XML attribute tags; its fields
			// are the same as ReportingIssue's, so a conversion suffices and a
			// field added to either side becomes a compile error here.
			st.Issues = append(st.Issues, ReportingIssue(i))
		}
		out.Reports = append(out.Reports, st)
	}
	return out, nil
}
