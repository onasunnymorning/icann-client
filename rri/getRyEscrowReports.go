package rri

import (
	"context"
	"encoding/xml"
	"time"
)

// ReceivedReport is one registry escrow report ICANN holds, with the moment
// ICANN received it. That timestamp is what the HEAD status check cannot give:
// it distinguishes a deposit that arrived on time from a late resend.
type ReceivedReport struct {
	Received time.Time  `json:"received"`
	Report   ReportMeta `json:"report"`
}

// EscrowReports lists the reports ICANN received for one TLD and watermark date.
type EscrowReports struct {
	TLD  string `json:"tld"`
	Date string `json:"date"` // YYYY-MM-DD
	// Reports is empty, never nil, when ICANN holds nothing for the date.
	Reports []ReceivedReport `json:"reports"`
}

// xmlReceivedReports is a decode-only mirror of <rdeReports:reports>.
type xmlReceivedReports struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:rdeReports-1.0 reports"`
	Items   []struct {
		Received string    `xml:"urn:ietf:params:xml:ns:rdeReports-1.0 received"`
		Report   xmlReport `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 report"`
	} `xml:"urn:ietf:params:xml:ns:rdeReports-1.0 receivedReport"`
}

// GetRyEscrowReports returns the registry escrow reports ICANN received for the
// client's TLD whose watermark falls on date, per
// draft-lozano-icann-registry-interfaces Section 2.3.1.
//
// Unlike GetRyEscrowReportStatus, which issues HEAD, this issues GET and
// decodes the body. ICANN serves GET only for dates after draft -27 reached
// production; an earlier date is rejected with result code 2214
// (ResultDateBeforeGET), returned as a *ResultError.
//
// HTTP 404 means ICANN holds no report for the date, or has purged it under
// its retention policy, and yields an empty list rather than an error.
func (c *Client) GetRyEscrowReports(ctx context.Context, date time.Time) (*EscrowReports, error) {
	req, raw, err := c.getEscrowMonitor(ctx, "registry-escrow-report", date)
	if err != nil {
		return nil, err
	}
	out := &EscrowReports{TLD: c.Config().TLD, Date: date.Format("2006-01-02"), Reports: []ReceivedReport{}}
	if raw == nil {
		return out, nil
	}

	var doc xmlReceivedReports
	if err := newXMLDecoder(raw).Decode(&doc); err != nil {
		return nil, decodeError(req, "a data escrow reports document", raw, err)
	}
	for _, it := range doc.Items {
		received, err := parseTimestamp("rdeReports:received", "the received report", it.Received)
		if err != nil {
			return nil, decodeError(req, "a data escrow reports document", raw, err)
		}
		meta, err := it.Report.meta()
		if err != nil {
			return nil, decodeError(req, "a data escrow reports document", raw, err)
		}
		out.Reports = append(out.Reports, ReceivedReport{Received: received, Report: *meta})
	}
	return out, nil
}
