package mosapi

import (
	base "github.com/onasunnymorning/icann-client/client"
)

// Client provides MOSAPI-specific helpers built on top of the shared client.
// For now, it simply composes the shared client; MOSAPI resource methods can
// be added here.
type Client struct {
	*base.Client
}

// New creates a MOSAPI client using the shared configuration and auth. This
// allows sharing credentials across MOSAPI and RRI clients.
func New(cfg base.Config) (*Client, error) {
	c, err := base.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{Client: c}, nil
}
