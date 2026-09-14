package mosapi

import (
	"sync"

	base "github.com/onasunnymorning/icann-client/client"
)

// Client provides MOSAPI-specific helpers built on top of the shared client.
//
// It owns the MOSAPI session: endpoint methods log in on demand, reuse the
// session cookie across calls, and renew it once if the server reports it
// expired. Because ICANN permits a single concurrent session per account, a
// Client is meant to be created once and reused; it is safe for concurrent use.
type Client struct {
	*base.Client

	// mu serializes session establishment and renewal. MOSAPI permits a single
	// concurrent session per account, so two goroutines must not log in at once.
	mu sync.Mutex
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
