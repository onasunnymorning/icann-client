package client

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a minimal HTTP client wrapper for ICANN APIs (MOSAPI/RRI).
// It encapsulates authentication and base URL selection based on environment.
type Client struct {
	// HTTPClient is the underlying HTTP client used for requests.
	HTTPClient *http.Client

	// baseURL is the root endpoint (differs by environment).
	baseURL *url.URL

	// cfg is the validated configuration used to construct the client.
	cfg Config
}

// NewClient constructs a new Client from the provided Config.
// It applies sensible defaults, validates the configuration, and configures
// authentication via either HTTP Basic or TLS client certificate ("TLSA").
func NewClient(cfg Config) (*Client, error) {
	// Apply defaults if not set
	if cfg.Version == "" {
		cfg.Version = V2
	}
	if cfg.Environment == "" {
		cfg.Environment = ENV_PROD
	}
	if cfg.Entity == "" {
		cfg.Entity = EntityRegistry
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Select base URL by environment
	rawBase := MOSAPI_URL
	if cfg.Environment == ENV_OTE {
		rawBase = MOSAPI_OTE_URL
	}
	u, err := url.Parse(rawBase)
	if err != nil {
		return nil, err
	}

	// Start with a cloned default transport for sane defaults
	var baseTransport *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		baseTransport = dt.Clone()
	} else {
		baseTransport = &http.Transport{}
	}

	var rt http.RoundTripper = baseTransport

	switch cfg.AuthType {
	case AUTH_TYPE_BASIC:
		rt = &basicAuthTransport{
			username: cfg.Username,
			password: cfg.Password,
			base:     baseTransport,
		}
	case AUTH_TYPE_TLSA:
		// Configure mutual TLS using provided PEM-encoded certificate and key
		cert, err := tls.X509KeyPair([]byte(cfg.CertificatePEM), []byte(cfg.KeyPEM))
		if err != nil {
			return nil, err
		}
		if baseTransport.TLSClientConfig == nil {
			baseTransport.TLSClientConfig = &tls.Config{}
		}
		baseTransport.TLSClientConfig.MinVersion = tls.VersionTLS12
		baseTransport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		rt = baseTransport
	}

	httpClient := &http.Client{
		Transport: rt,
		Timeout:   30 * time.Second,
	}

	return &Client{
		HTTPClient: httpClient,
		baseURL:    u,
		cfg:        cfg,
	}, nil
}

// NewRequest builds an HTTP request relative to the client's base URL.
// Path may be absolute (http...) or relative (e.g. "/v2/...").
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	var fullURL *url.URL
	p, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	if p.IsAbs() {
		fullURL = p
	} else {
		fullURL = c.baseURL.ResolveReference(p)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// Do executes an HTTP request using the underlying HTTP client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.HTTPClient.Do(req)
}

// WithBaseURL overrides the client's base URL (useful for tests or custom endpoints).
func (c *Client) WithBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	c.baseURL = u
	return nil
}

// Config returns a copy of the validated configuration used to construct the client.
func (c *Client) Config() Config { return c.cfg }
