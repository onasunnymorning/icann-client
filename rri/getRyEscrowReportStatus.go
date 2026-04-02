package rri

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	rawBase := base.RRI_URL
	if cfg.Environment == base.ENV_OTE {
		rawBase = base.RRI_OTE_URL
	}
	if err := c.WithBaseURL(rawBase); err != nil {
		return nil, err
	}

	return &Client{Client: c}, nil
}

// GetRyEscrowReportStatus checks the status of the Ry Escrow report for the client's TLD and the given date.
// Per draft: HEAD will return 200 if available, 404 if not available.
func (c *Client) GetRyEscrowReportStatus(ctx context.Context, date time.Time) (*ReportStatus, error) {
	cfg := c.Config()
	// Using the path specified in draft-lozano-icann-registry-interfaces Section 2.3.1
	// <base-url>/info/report/registry-escrow-report/<tld>/<date>
	path := fmt.Sprintf("/info/report/registry-escrow-report/%s/%s", cfg.TLD, date.Format("2006-01-02"))
	// Use HEAD as required by the IETF draft Section 2.3 for monitoring endpoints
	req, err := c.NewRequest(ctx, http.MethodHead, path, nil)
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
		b, _ := io.ReadAll(resp.Body)
		bodyStr := string(b)
		// The API may return 404 for unauthorized or invalid paths with an HTML Apache error page, 
		// or a JSON error message. A genuine "pending" state should ideally not be a server error page.
		if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || strings.Contains(strings.ToLower(bodyStr), "<html") {
			return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: bodyStr}
		}
		// If we receive a JSON response that indicates an error (like unauthorized), fail as well.
		// A generic json with an error code usually has "code" or "message" but we don't have a strict schema here.
		if strings.Contains(strings.ToLower(bodyStr), "unauthorized") || strings.Contains(strings.ToLower(bodyStr), "error") {
			return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: bodyStr}
		}

		rs.Status = RY_RDEReport_PENDING
		return rs, nil
	default:
		b, _ := io.ReadAll(resp.Body)
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: string(b)}
	}
}
