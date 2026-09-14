package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestNewClient_BasicAuthTransport(t *testing.T) {
	cfg := Config{
		TLD:         "example",
		AuthType:    AUTH_TYPE_BASIC,
		Username:    "alice",
		Password:    "secret",
		Version:     V2,
		Entity:      EntityRegistry,
		Environment: ENV_PROD,
	}

	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	// Start a test server to inspect headers
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	resp.Body.Close()

	if gotAuth == "" {
		t.Fatalf("expected Authorization header to be set for basic auth")
	}
}

func TestNewClient_TLSACertLoaded(t *testing.T) {
	// Generate a private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	// Create a template for certificate
	tmpl := x509.Certificate{
		SerialNumber:          bigIntOne(),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	// Build PEM strings
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	cfg := Config{
		TLD:            "example",
		AuthType:       AUTH_TYPE_TLSA,
		CertificatePEM: string(certPEM),
		KeyPEM:         string(keyPEM),
		Version:        V2,
		Entity:         EntityRegistry,
		Environment:    ENV_PROD,
	}

	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.HTTPClient.Transport)
	}
	if tr.TLSClientConfig == nil || len(tr.TLSClientConfig.Certificates) == 0 {
		t.Fatalf("expected TLS client certificate to be configured")
	}
}

// bigIntOne returns big.Int(1) to avoid importing math/big in multiple places.
func bigIntOne() *big.Int { return big.NewInt(1) }

// TestNewClientHasCookieJar guards a requirement of MOSAPI, which is session
// based: /login returns a session cookie that every other endpoint expects. A
// client without a jar silently drops it, and those endpoints answer 401 no
// matter how correct the credentials are.
func TestNewClientHasCookieJar(t *testing.T) {
	c, err := NewClient(Config{TLD: "example", AuthType: AUTH_TYPE_BASIC, Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.HTTPClient.Jar == nil {
		t.Fatal("HTTPClient.Jar is nil; MOSAPI session cookies would be dropped")
	}

	u, err := url.Parse("https://mosapi.icann.org/ry/example/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	c.HTTPClient.Jar.SetCookies(u, []*http.Cookie{{Name: "id", Value: "abc", Path: "/ry/example"}})
	got := c.HTTPClient.Jar.Cookies(u)
	if len(got) != 1 || got[0].Name != "id" || got[0].Value != "abc" {
		t.Errorf("cookies = %v, want the session cookie to round-trip", got)
	}
}

// TestBaseURLIsACopy keeps callers from mutating the client's base URL through
// the accessor.
func TestBaseURLIsACopy(t *testing.T) {
	c, err := NewClient(Config{TLD: "example", AuthType: AUTH_TYPE_BASIC, Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	got := c.BaseURL()
	if got.String() != MOSAPI_URL {
		t.Errorf("BaseURL() = %q, want %q", got, MOSAPI_URL)
	}
	got.Path = "/mutated"
	if c.BaseURL().Path != "" {
		t.Errorf("BaseURL() returned a reference; the client's URL was mutated to %q", c.BaseURL().Path)
	}
}
