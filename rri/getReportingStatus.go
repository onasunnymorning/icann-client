package rri

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// The reporting obligations ICANN tracks per TLD, as returned in the "path"
// field. These are the spellings production uses; they are not the element
// names in draft-lozano-icann-registry-interfaces, which describes a different
// document for this endpoint than ICANN actually serves.
const (
	ReportingPathFull     = "Full" // full registry escrow deposit
	ReportingPathDiff     = "Diff" // differential registry escrow deposit
	ReportingPathDea      = "Dea"  // data escrow agent notification
	ReportingPathPRTR     = "PRTR" // per-registrar transactions report (Specification 3 Section 1)
	ReportingPathRFAR     = "RFAR" // registry functions activity report (Specification 3 Section 2)
	ReportingPathRegistry = "Registry"
)

// ReportingStatusOK is the status of an obligation ICANN has no complaint
// about. Any other value is a complaint; ICANN does not publish the set.
const ReportingStatusOK = "ok"

// ReportingPath is ICANN's current view of one reporting obligation.
type ReportingPath struct {
	Path   string `json:"path"`   // one of the ReportingPath* constants
	Status string `json:"status"` // "ok", or a complaint
}

// ReportingSummary is ICANN's reporting status for a TLD.
//
// It is a snapshot, not a history: Created is the moment ICANN generated the
// response, and each Status describes the obligation as of then. The response
// carries no date range and no per-date detail, so it reports whether an
// obligation is currently satisfied but not which periods were ever missed.
type ReportingSummary struct {
	TLD     string          `json:"tld"`
	Paths   []ReportingPath `json:"paths"`
	Created string          `json:"created"` // when ICANN generated this snapshot
}

// Unsatisfactory returns the reporting obligations ICANN is not currently
// satisfied with. An empty result means the TLD is square with ICANN as of
// Created.
func (s *ReportingSummary) Unsatisfactory() []ReportingPath {
	var out []ReportingPath
	for _, p := range s.Paths {
		if p.Status != ReportingStatusOK {
			out = append(out, p)
		}
	}
	return out
}

// jsonReportingSummary is a decode-only mirror of what ICANN sends. It exists
// because the TLD arrives nested as {"tld": {"name": "..."}}, which is not a
// shape worth exposing to callers.
type jsonReportingSummary struct {
	TLD struct {
		Name string `json:"name"`
	} `json:"tld"`
	Paths   []ReportingPath `json:"paths"`
	Created string          `json:"created"`
}

// GetReportingStatus returns ICANN's current view of the client's TLD
// reporting: each obligation and whether ICANN is satisfied with it.
//
// Note that ICANN serves JSON here, not the XML document described by
// draft-lozano-icann-registry-interfaces Section 6. Production refuses
// "Accept: text/xml" on this endpoint with HTTP 406, so no Accept header is
// sent and the response is decoded as JSON.
//
// The answer is a snapshot with no dates in it, so it says whether a TLD is
// currently square with ICANN, not which reporting periods were missed. Use
// GetMonthlyReportStatus or GetRyEscrowReportStatus to ask about a period.
func (c *Client) GetReportingStatus(ctx context.Context) (*ReportingSummary, error) {
	cfg := c.Config()
	path := fmt.Sprintf("/info/status/registry/%s", url.PathEscape(cfg.TLD))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	// No Accept header: the draft requires none, and ICANN answers 406 to
	// "Accept: text/xml" here because it cannot produce XML for this resource.

	raw, _, err := c.doGet(req)
	if err != nil {
		return nil, err
	}

	var doc jsonReportingSummary
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, decodeError(req, "a reporting summary", raw, err)
	}
	return &ReportingSummary{
		TLD:     doc.TLD.Name,
		Paths:   doc.Paths,
		Created: doc.Created,
	}, nil
}
