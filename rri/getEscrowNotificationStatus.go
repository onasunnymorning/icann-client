package rri

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NamespaceRRIReporting is the XML namespace of the reporting summary.
const NamespaceRRIReporting = "urn:ietf:params:xml:ns:rriReporting-1.0"

// ReportingTypeDEANotification is the <rriReporting:type> of the one status
// report in the escrow agent notification summary.
const ReportingTypeDEANotification = "DEA_Notification"

// Issue descriptions defined by the draft's rriReporting schema.
const (
	IssueMissingDepositFull = "Missing_Deposit_Full"
	IssueMissingDepositDiff = "Missing_Deposit_Diff"
	IssueInvalidDepositFull = "Invalid_Deposit_Full"
	IssueInvalidDepositDiff = "Invalid_Deposit_Diff"
	IssueNoReportReceived   = "No_Report_Received"
)

// NotificationIssue is one date for which ICANN is not satisfied.
type NotificationIssue struct {
	Date        string `json:"date"`        // YYYY-MM-DD
	Description string `json:"description"` // one of the Issue* constants
}

// EscrowNotificationStatus is ICANN's view of the data escrow agent
// notifications for a TLD, per draft-lozano-icann-registry-interfaces
// Section 6.2.
//
// Where GetEscrowNotifications lists what ICANN received on a date, this is the
// state after ICANN processed it: whether each day's notification and deposit
// were satisfactory. It is a snapshot; Created is when ICANN generated it.
//
// Only Created, Status and TLD are populated when ICANN answers in the JSON
// shape it uses for the registry reporting status. The remaining fields come
// from the XML document the draft specifies.
type EscrowNotificationStatus struct {
	TLD             string              `json:"tld"`
	Created         string              `json:"created"`
	Status          string              `json:"status"` // "ok" or "unsatisfactory"
	Enabled         *bool               `json:"enabled,omitempty"`
	DepositSchedule string              `json:"depositSchedule,omitempty"` // None, Weekly or Daily
	LastFullDate    string              `json:"lastFullDate,omitempty"`    // last validated FULL watermark
	Issues          []NotificationIssue `json:"issues"`
}

// xmlEscrowNotificationStatus is a decode-only mirror of <rriReporting:summary>.
type xmlEscrowNotificationStatus struct {
	XMLName         xml.Name `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 summary"`
	TLD             string   `xml:"urn:ietf:params:xml:ns:rdeHeader-1.0 tld"`
	CreationDate    string   `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 creationDate"`
	DepositSchedule string   `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 depositSchedule"`
	LastFullDate    string   `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 lastFullDate"`
	StatusReports   []struct {
		Type    string `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 type"`
		Enabled string `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 enabled"`
		Status  string `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 status"`
		Issues  []struct {
			Date        string `xml:"date,attr"`
			Description string `xml:"description,attr"`
		} `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 issues>issue"`
	} `xml:"urn:ietf:params:xml:ns:rriReporting-1.0 statusReports>statusReport"`
}

// GetEscrowNotificationStatus returns ICANN's reporting status for the escrow
// agent notifications of the client's TLD, per
// draft-lozano-icann-registry-interfaces Section 6.2.
//
// The draft describes this interface for data escrow agents, so credentials
// issued to a registry operator may be refused; that surfaces as the HTTP
// error ICANN sent.
//
// The draft specifies XML, but ICANN serves JSON on the sibling registry
// status interface (see GetReportingStatus). This one has not been observed in
// production, so the body is decoded as whichever of the two it is, chosen by
// its first character. A body matching neither is reported with the payload
// attached.
func (c *Client) GetEscrowNotificationStatus(ctx context.Context) (*EscrowNotificationStatus, error) {
	path := fmt.Sprintf("/info/status/escrow-agent-notification/%s", url.PathEscape(c.Config().TLD))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	// No Accept header, for the reason given in GetReportingStatus.

	raw, _, err := c.doGet(req)
	if err != nil {
		return nil, err
	}

	const what = "an escrow notification status document"
	var out *EscrowNotificationStatus
	if bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(raw, utf8BOM)), []byte("{")) {
		out, err = decodeJSONNotificationStatus(raw)
	} else {
		out, err = decodeXMLNotificationStatus(raw)
	}
	if err != nil {
		return nil, decodeError(req, what, raw, err)
	}
	return out, nil
}

// decodeJSONNotificationStatus reads the shape of the registry reporting
// status ({"tld":{"name"},"paths":[{"path","status"}],"created"}), taking the
// status from the Dea path.
func decodeJSONNotificationStatus(raw []byte) (*EscrowNotificationStatus, error) {
	var doc jsonReportingSummary
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := &EscrowNotificationStatus{TLD: doc.TLD.Name, Created: doc.Created, Issues: []NotificationIssue{}}
	for _, p := range doc.Paths {
		if strings.EqualFold(p.Path, ReportingPathDea) {
			out.Status = p.Status
		}
	}
	if out.Status == "" {
		return nil, fmt.Errorf("no %q path in the response", ReportingPathDea)
	}
	return out, nil
}

func decodeXMLNotificationStatus(raw []byte) (*EscrowNotificationStatus, error) {
	var doc xmlEscrowNotificationStatus
	if err := newXMLDecoder(raw).Decode(&doc); err != nil {
		return nil, err
	}
	out := &EscrowNotificationStatus{
		TLD:             strings.TrimSpace(doc.TLD),
		Created:         strings.TrimSpace(doc.CreationDate),
		DepositSchedule: strings.TrimSpace(doc.DepositSchedule),
		LastFullDate:    strings.TrimSpace(doc.LastFullDate),
		Issues:          []NotificationIssue{},
	}
	for _, sr := range doc.StatusReports {
		if strings.TrimSpace(sr.Type) != ReportingTypeDEANotification {
			continue
		}
		out.Status = strings.TrimSpace(sr.Status)
		if e := strings.TrimSpace(sr.Enabled); e != "" {
			b := strings.EqualFold(e, "true") || e == "1"
			out.Enabled = &b
		}
		for _, is := range sr.Issues {
			out.Issues = append(out.Issues, NotificationIssue{Date: is.Date, Description: is.Description})
		}
	}
	if out.Status == "" {
		return nil, fmt.Errorf("no %s status report in the summary", ReportingTypeDEANotification)
	}
	if out.Created != "" {
		if _, err := time.Parse(time.RFC3339, out.Created); err != nil {
			return nil, fmt.Errorf("rriReporting:creationDate: cannot parse %q as a timestamp", out.Created)
		}
	}
	return out, nil
}
