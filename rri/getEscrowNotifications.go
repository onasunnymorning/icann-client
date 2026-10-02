package rri

import (
	"context"
	"encoding/xml"
	"strings"
	"time"
)

// ReceivedNotification is one data escrow agent notification ICANN received,
// with the moment ICANN received it.
type ReceivedNotification struct {
	Received time.Time `json:"received"`
	DeaName  string    `json:"deaName"`
	Version  int       `json:"version"`
	RepDate  string    `json:"repDate"` // YYYY-MM-DD
	// Status is the agent's verdict, e.g. DVPN (deposit verified) or DRFN (no
	// new deposit received; lastFullDate is the last FULL the agent holds).
	Status       string     `json:"status"`
	REDate       *time.Time `json:"reDate,omitempty"`
	VADate       *time.Time `json:"vaDate,omitempty"`
	LastFullDate string     `json:"lastFullDate,omitempty"` // YYYY-MM-DD
	// Report is the deposit the notification is about; absent for a DRFN.
	Report *ReportMeta `json:"report,omitempty"`
}

// EscrowNotifications lists the agent notifications ICANN received for one TLD
// and report date.
type EscrowNotifications struct {
	TLD  string `json:"tld"`
	Date string `json:"date"` // YYYY-MM-DD
	// Notifications is empty, never nil, when ICANN holds nothing for the date.
	Notifications []ReceivedNotification `json:"notifications"`
}

// xmlReceivedNotifications is a decode-only mirror of
// <rdeNotifications:notifications>.
type xmlReceivedNotifications struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:rdeNotifications-1.0 notifications"`
	Items   []struct {
		Received     string `xml:"urn:ietf:params:xml:ns:rdeNotifications-1.0 received"`
		Notification struct {
			DeaName      string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 deaName"`
			Version      int        `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 version"`
			RepDate      string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 repDate"`
			Status       string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 status"`
			REDate       string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 reDate"`
			VADate       string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 vaDate"`
			LastFullDate string     `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 lastFullDate"`
			Report       *xmlReport `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 report"`
		} `xml:"urn:ietf:params:xml:ns:rdeNotification-1.0 notification"`
	} `xml:"urn:ietf:params:xml:ns:rdeNotifications-1.0 receivedNotification"`
}

// GetEscrowNotifications returns the data escrow agent notifications ICANN
// received for the client's TLD on date, per
// draft-lozano-icann-registry-interfaces Section 2.3.2. It tells a registry
// operator whether its escrow agent has verified the deposit, which the
// deposit status check alone cannot.
//
// Error behaviour matches GetRyEscrowReports: HTTP 404 yields an empty list,
// and a date before draft -27 reached production is a *ResultError with code
// ResultDateBeforeGET.
func (c *Client) GetEscrowNotifications(ctx context.Context, date time.Time) (*EscrowNotifications, error) {
	req, raw, err := c.getEscrowMonitor(ctx, "escrow-agent-notification", date)
	if err != nil {
		return nil, err
	}
	out := &EscrowNotifications{TLD: c.Config().TLD, Date: date.Format("2006-01-02"), Notifications: []ReceivedNotification{}}
	if raw == nil {
		return out, nil
	}

	const what = "a data escrow notifications document"
	var doc xmlReceivedNotifications
	if err := newXMLDecoder(raw).Decode(&doc); err != nil {
		return nil, decodeError(req, what, raw, err)
	}
	for _, it := range doc.Items {
		n := it.Notification
		rn := ReceivedNotification{
			DeaName:      strings.TrimSpace(n.DeaName),
			Version:      n.Version,
			RepDate:      strings.TrimSpace(n.RepDate),
			Status:       strings.ToUpper(strings.TrimSpace(n.Status)),
			LastFullDate: strings.TrimSpace(n.LastFullDate),
		}
		var err error
		if rn.Received, err = parseTimestamp("rdeNotifications:received", "the received notification", it.Received); err != nil {
			return nil, decodeError(req, what, raw, err)
		}
		// reDate and vaDate are optional: a DRFN has neither.
		for _, f := range []struct {
			name string
			in   string
			dst  **time.Time
		}{{"rdeNotification:reDate", n.REDate, &rn.REDate}, {"rdeNotification:vaDate", n.VADate, &rn.VADate}} {
			if strings.TrimSpace(f.in) == "" {
				continue
			}
			t, err := parseTimestamp(f.name, "the notification", f.in)
			if err != nil {
				return nil, decodeError(req, what, raw, err)
			}
			*f.dst = &t
		}
		if n.Report != nil {
			meta, err := n.Report.meta()
			if err != nil {
				return nil, decodeError(req, what, raw, err)
			}
			rn.Report = meta
		}
		out.Notifications = append(out.Notifications, rn)
	}
	return out, nil
}
