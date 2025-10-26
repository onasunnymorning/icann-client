package rri

import (
	"context"
	"fmt"
	"net/http"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// ReportStatus represents the status of a registry escrow (Ry Escrow) report for a given date.
type ReportStatus struct {
	Type   string    // e.g. "ry-escrow"
	TLD    string    // e.g. "example"
	Date   time.Time // date of report
	Status string    // one of RY_RDEReport_RECEIVED or RY_RDEReport_PENDING
}

const (
	RY_RDEReport_RECEIVED = "received"
	RY_RDEReport_PENDING  = "pending"
)

// Client provides RRI-specific helpers built on top of the shared client.
type Client struct{ *base.Client }

// New creates an RRI client using the shared configuration and auth.
func New(cfg base.Config) (*Client, error) {
	c, err := base.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{Client: c}, nil
}

// GetRyEscrowReportStatus checks the status of the Ry Escrow report for the client's TLD and the given date.
// Per draft: HEAD will return 200 if available, 404 if not available.
func (c *Client) GetRyEscrowReportStatus(ctx context.Context, date time.Time) (*ReportStatus, error) {
	cfg := c.Config()
	// Construct a reasonable path; adjust to spec as needed when finalized.
	// Using an RRI-scoped path independent of MOSAPI entity/version routing.
	path := fmt.Sprintf("/rri/escrow/ry/%s/%s/status", cfg.TLD, date.Format("2006-01-02"))
	// Use GET instead of HEAD to avoid noisy http2 client logs when servers
	// incorrectly send DATA on a HEAD response (observed in the wild).
	// We only inspect the status code and ignore the body.
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rs := &ReportStatus{Type: "ry-escrow", TLD: cfg.TLD, Date: date}
	switch resp.StatusCode {
	case http.StatusOK:
		rs.Status = RY_RDEReport_RECEIVED
		return rs, nil
	case http.StatusNotFound:
		rs.Status = RY_RDEReport_PENDING
		return rs, nil
	default:
		return nil, fmt.Errorf("unexpected status code: %d for %s %s", resp.StatusCode, req.Method, req.URL.String())
	}
}
