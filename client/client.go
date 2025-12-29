package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/json"
	"errors"
	"fmt"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/youmark/pkcs8"
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
		// Handle encrypted private keys by attempting to decrypt them
		keyPEM := cfg.KeyPEM
		if strings.Contains(keyPEM, "ENCRYPTED") {
			// Try to decrypt the encrypted private key using the provided passphrase
			decryptedKey, err := decryptPrivateKey(keyPEM, cfg.KeyPassphrase)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt private key (encrypted keys require a passphrase): %w", err)
			}
			keyPEM = decryptedKey
		}
		cert, err := tls.X509KeyPair([]byte(cfg.CertificatePEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("failed to parse TLS certificate/key pair: %w", err)
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

// decryptPrivateKey attempts to decrypt an encrypted PEM-encoded private key.
// Supports both RFC 1423 (legacy) and PKCS#8 encrypted keys.
// If passphrase is empty, it will attempt to decrypt without a passphrase (which will fail for encrypted keys).
// Returns the decrypted PEM-encoded key, or an error if decryption fails.
func decryptPrivateKey(encryptedPEM, passphrase string) (string, error) {
	block, _ := pem.Decode([]byte(encryptedPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}

	// Check for PKCS#8 encrypted private key (ENCRYPTED PRIVATE KEY)
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		if passphrase == "" {
			return "", fmt.Errorf("PKCS#8 encrypted private key requires a passphrase")
		}

		// Decrypt PKCS#8 encrypted key
		key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, []byte(passphrase))
		if err != nil {
			return "", fmt.Errorf("failed to decrypt PKCS#8 encrypted private key: %w", err)
		}

		// Determine the key type and marshal it appropriately
		var keyBytes []byte
		var keyType string

		switch k := key.(type) {
		case *rsa.PrivateKey:
			keyBytes = x509.MarshalPKCS1PrivateKey(k)
			keyType = "RSA PRIVATE KEY"
		case *ecdsa.PrivateKey:
			var err error
			keyBytes, err = x509.MarshalECPrivateKey(k)
			if err != nil {
				return "", fmt.Errorf("failed to marshal ECDSA private key: %w", err)
			}
			keyType = "EC PRIVATE KEY"
		default:
			// For other key types, try to marshal as PKCS#8
			keyBytes, err = x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				return "", fmt.Errorf("unsupported private key type or failed to marshal: %w", err)
			}
			keyType = "PRIVATE KEY"
		}

		// Re-encode as unencrypted PEM
		block = &pem.Block{
			Type:  keyType,
			Bytes: keyBytes,
		}
		return string(pem.EncodeToMemory(block)), nil
	}

	// Check for RFC 1423 encrypted PEM block (legacy format)
	if x509.IsEncryptedPEMBlock(block) {
		// Attempt to decrypt using RFC 1423
		der, err := x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return "", fmt.Errorf("decryption failed (encrypted keys require a passphrase): %w", err)
		}

		// Determine the key type from the original block type
		keyType := "PRIVATE KEY"
		if strings.Contains(block.Type, "RSA") {
			keyType = "RSA PRIVATE KEY"
		} else if strings.Contains(block.Type, "EC") {
			keyType = "EC PRIVATE KEY"
		}

		// Re-encode as unencrypted PEM
		block = &pem.Block{
			Type:  keyType,
			Bytes: der,
		}
		return string(pem.EncodeToMemory(block)), nil
	}

	// Not encrypted, return as-is
	return encryptedPEM, nil
}

// DoJSON issues an HTTP request with optional JSON body and decodes a JSON response into out.
// If in is non-nil, it will be JSON-encoded and sent with Content-Type: application/json.
// If out is non-nil and the response has a JSON Content-Type, it will be decoded.
// Returns a *HTTPError for non-2xx responses.
func (c *Client) DoJSON(ctx context.Context, method, path string, in any, out any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(in); err != nil {
			return nil, err
		}
		body = buf
	}
	req, err := c.NewRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain and close body to allow connection reuse
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp, &HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	if out != nil {
		// Best-effort JSON decode; handle empty body
		decErr := json.NewDecoder(resp.Body).Decode(out)
		if decErr != nil {
			// If the body is empty (EOF before any bytes), tolerate when out is non-nil by returning a clearer error
			if errors.Is(decErr, io.EOF) {
				// nothing to decode; treat as success
				return resp, nil
			}
			resp.Body.Close()
			return resp, decErr
		}
	}
	return resp, nil
}
