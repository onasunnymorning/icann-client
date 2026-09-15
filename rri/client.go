package rri

import (
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// Client provides RRI-specific helpers built on top of the shared client.
type Client struct{ *base.Client }

// New creates an RRI client using the shared configuration and auth.
//
// The shared client defaults its base URL to MOSAPI, so New overrides it with
// the RRI endpoint for the configured environment. RRI authenticates every
// request on its own and has no login step, unlike MOSAPI.
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

// ReportStatus represents the status of a report ICANN may or may not have
// received. It is shared by the escrow and monthly status endpoints.
type ReportStatus struct {
	Type   string    // e.g. "ry-escrow"
	TLD    string    // e.g. "example"
	Date   time.Time // date of report
	Status string    // one of RY_RDEReport_RECEIVED or RY_RDEReport_PENDING
}

const (
	// RY_RDEReport_RECEIVED means ICANN has the report.
	RY_RDEReport_RECEIVED = "received"
	// RY_RDEReport_PENDING means ICANN has not received the report yet.
	RY_RDEReport_PENDING = "pending"
)
