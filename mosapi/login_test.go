package mosapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// sessionServer is a MOSAPI stand-in that implements the session dance: /login
// mints a cookie, and the endpoint handler only runs once a valid cookie is
// presented. Tests assert against its recorded traffic.
type sessionServer struct {
	mu sync.Mutex

	// Paths records every request path in order, including logins.
	Paths []string
	// Logins counts successful /login calls.
	Logins int
	// Logouts counts /logout calls.
	Logouts int
	// CookiesSeen records the session cookie presented on each endpoint call,
	// with "" for a call that carried none.
	CookiesSeen []string

	// RejectLogin makes /login answer 401, as a wrong password would.
	RejectLogin bool
	// OmitCookie makes /login answer 200 without setting a cookie.
	OmitCookie bool
	// ExpireSessions makes every endpoint call answer 401, as an expired or
	// evicted session would.
	ExpireSessions bool
	// ExpireOnce makes only the first endpoint call answer 401.
	ExpireOnce bool

	issued  int
	current string
	handler http.HandlerFunc
}

func (s *sessionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.Paths = append(s.Paths, r.URL.Path)

	switch r.URL.Path {
	case "/ry/example/login":
		if s.RejectLogin {
			s.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.Logins++
		if s.OmitCookie {
			s.mu.Unlock()
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, "Login successful")
			return
		}
		s.issued++
		s.current = fmt.Sprintf("session-%d", s.issued)
		value := s.current
		s.mu.Unlock()
		// Path scoping and httpOnly match what MOSAPI sets; Secure is omitted
		// because the test server speaks plain HTTP.
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: value, Path: "/ry/example", HttpOnly: true})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "Login successful")
		return

	case "/ry/example/logout":
		s.Logouts++
		s.current = ""
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "Logout successful")
		return
	}

	var presented string
	if ck, err := r.Cookie(sessionCookieName); err == nil {
		presented = ck.Value
	}
	s.CookiesSeen = append(s.CookiesSeen, presented)
	expire := s.ExpireSessions || (s.ExpireOnce && len(s.CookiesSeen) == 1)
	valid := presented != "" && presented == s.current
	s.mu.Unlock()

	if expire || !valid {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("The client could not be authenticated using any of the available methods: TLS-Client-Authentication or Session Cookie."))
		return
	}
	s.handler(w, r)
}

// newSessionClient returns a client pointed at a sessionServer wrapping handler.
func newSessionClient(t *testing.T, handler http.HandlerFunc) (*Client, *sessionServer) {
	t.Helper()
	ss := &sessionServer{handler: handler}
	srv := httptest.NewServer(ss)
	t.Cleanup(srv.Close)

	c, err := New(base.Config{
		TLD: "example", Environment: base.ENV_PROD, Version: base.V2,
		Entity: base.EntityRegistry, AuthType: base.AUTH_TYPE_BASIC,
		Username: "u", Password: "p",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.WithBaseURL(srv.URL); err != nil {
		t.Fatalf("WithBaseURL() error = %v", err)
	}
	return c, ss
}

func stateHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"tld":"example","status":"Up","testedServices":{},"version":2}`))
}

// TestLoginPrecedesEndpointCall is the regression test for the bug this fixes:
// MOSAPI accepts credentials only at /login, so a versioned endpoint reached
// without a session cookie answers 401 no matter how correct the password is.
func TestLoginPrecedesEndpointCall(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)

	if _, err := c.GetStateResponse(context.Background()); err != nil {
		t.Fatalf("GetStateResponse() error = %v", err)
	}

	want := []string{"/ry/example/login", "/ry/example/v2/monitoring/state"}
	if len(ss.Paths) != len(want) || ss.Paths[0] != want[0] || ss.Paths[1] != want[1] {
		t.Fatalf("request paths = %v, want %v", ss.Paths, want)
	}
	// The login endpoint is deliberately unversioned; a "/v2/login" would 404.
	if strings.Contains(ss.Paths[0], "/"+base.V2+"/") {
		t.Errorf("login path %q must not carry the API version", ss.Paths[0])
	}
	if ss.CookiesSeen[0] == "" {
		t.Error("endpoint call carried no session cookie")
	}
	if !c.HasSession() {
		t.Error("HasSession() = false after a successful login")
	}
}

// TestSessionReusedAcrossCalls guards the rule that matters operationally:
// MOSAPI permits one concurrent session per account and evicts the oldest when
// a new one is created, so a client must log in once and reuse it.
func TestSessionReusedAcrossCalls(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)

	for i := 0; i < 3; i++ {
		if _, err := c.GetStateResponse(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	if ss.Logins != 1 {
		t.Errorf("logged in %d times for 3 calls, want exactly 1", ss.Logins)
	}
	first := ss.CookiesSeen[0]
	for i, got := range ss.CookiesSeen {
		if got != first {
			t.Errorf("call %d used cookie %q, want the same session %q throughout", i+1, got, first)
		}
	}
}

// TestSessionRenewedOnce covers the expiry path: sessions last 15 minutes, so a
// 401 on a previously working session is routine and must be recovered from.
func TestSessionRenewedOnce(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)
	ss.ExpireOnce = true

	got, err := c.GetStateResponse(context.Background())
	if err != nil {
		t.Fatalf("GetStateResponse() error = %v, want the call to survive an expired session", err)
	}
	if got.Status != "Up" {
		t.Errorf("Status = %q, want Up", got.Status)
	}
	if ss.Logins != 2 {
		t.Errorf("logins = %d, want 2 (initial plus one renewal)", ss.Logins)
	}
	if len(ss.CookiesSeen) != 2 || ss.CookiesSeen[0] == ss.CookiesSeen[1] {
		t.Errorf("cookies presented = %v, want two different sessions", ss.CookiesSeen)
	}
}

// TestSessionRenewedAtMostOnce pins the retry as bounded. A 401 that survives a
// fresh login is a real authentication failure, not an expiry, and must be
// reported rather than retried forever.
func TestSessionRenewedAtMostOnce(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)
	ss.ExpireSessions = true

	_, err := c.GetStateResponse(context.Background())
	if err == nil {
		t.Fatal("GetStateResponse() = nil error, want a 401")
	}
	if ss.Logins != 2 {
		t.Errorf("logins = %d, want exactly 2; the retry must not loop", ss.Logins)
	}
	if len(ss.CookiesSeen) != 2 {
		t.Errorf("endpoint calls = %d, want 2", len(ss.CookiesSeen))
	}
}

// TestLoginFailureIsReportedBeforeAnyEndpointCall keeps a wrong password
// legible: it should name the credentials, not surface as a puzzling 401 from
// an endpoint that was never reached.
func TestLoginFailureIsReportedBeforeAnyEndpointCall(t *testing.T) {
	c, ss := newSessionClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("endpoint was called despite a failed login")
	})
	ss.RejectLogin = true

	_, err := c.GetStateResponse(context.Background())
	if err == nil {
		t.Fatal("GetStateResponse() = nil error, want a login failure")
	}
	if !strings.Contains(err.Error(), "username and password") {
		t.Errorf("error = %v, want it to point at the credentials", err)
	}
	if len(ss.Paths) != 1 {
		t.Errorf("requests = %v, want only the login attempt", ss.Paths)
	}
}

// TestLoginWithoutCookieIsAnError catches a 200 that carries no session: left
// alone it would send every later request unauthenticated, which reads as a
// credential problem rather than the protocol violation it is.
func TestLoginWithoutCookieIsAnError(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)
	ss.OmitCookie = true

	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("Login() = nil error, want an error when no session cookie is set")
	}
	if !strings.Contains(err.Error(), sessionCookieName) {
		t.Errorf("error = %v, want it to name the missing cookie", err)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	c, ss := newSessionClient(t, stateHandler)

	if _, err := c.GetStateResponse(context.Background()); err != nil {
		t.Fatalf("GetStateResponse() error = %v", err)
	}
	if err := c.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if c.HasSession() {
		t.Error("HasSession() = true after Logout()")
	}
	if ss.Logouts != 1 {
		t.Errorf("logouts = %d, want 1", ss.Logouts)
	}

	// A logout on a client holding no session must not spend a request.
	if err := c.Logout(context.Background()); err != nil {
		t.Fatalf("second Logout() error = %v", err)
	}
	if ss.Logouts != 1 {
		t.Errorf("logouts = %d after a no-op Logout(), want 1", ss.Logouts)
	}

	// The next call re-establishes a session rather than failing.
	if _, err := c.GetStateResponse(context.Background()); err != nil {
		t.Fatalf("GetStateResponse() after logout: %v", err)
	}
	if ss.Logins != 2 {
		t.Errorf("logins = %d, want 2", ss.Logins)
	}
}

// TestTLSAAuthSkipsLogin pins the other half of the 401 message: a TLS client
// certificate is accepted by the versioned endpoints directly, so a login there
// would burn a request and, worse, evict the account's session for no reason.
func TestTLSAAuthSkipsLogin(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)

	ss := &sessionServer{handler: stateHandler}
	srv := httptest.NewServer(ss)
	t.Cleanup(srv.Close)

	c, err := New(base.Config{
		TLD: "example", Environment: base.ENV_PROD, Version: base.V2,
		Entity: base.EntityRegistry, AuthType: base.AUTH_TYPE_TLSA,
		CertificatePEM: certPEM, KeyPEM: keyPEM,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.WithBaseURL(srv.URL); err != nil {
		t.Fatalf("WithBaseURL() error = %v", err)
	}

	// The stand-in server rejects a cookie-less call, which is all this needs:
	// the assertion is about which requests were issued, not about the outcome.
	_, _ = c.GetStateResponse(context.Background())

	if ss.Logins != 0 {
		t.Errorf("logins = %d, want 0 for certificate authentication", ss.Logins)
	}
	for _, p := range ss.Paths {
		if strings.HasSuffix(p, "/login") {
			t.Errorf("request paths = %v, want no login", ss.Paths)
		}
	}
}

// selfSignedPEM returns an ephemeral certificate and key, only so that a TLSA
// client can be constructed; no TLS handshake uses them here.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey() error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

// TestMetricaUsesSession checks the session path is wired into every MOSAPI
// endpoint, not only monitoring/state.
func TestMetricaUsesSession(t *testing.T) {
	c, ss := newSessionClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":2,"tld":"example","domainListDate":"2025-01-01","uniqueAbuseDomains":0,"domainListData":[]}`))
	})

	if _, err := c.GetMetricaLatest(context.Background()); err != nil {
		t.Fatalf("GetMetricaLatest() error = %v", err)
	}
	if _, err := c.GetMetricaByDate(context.Background(), "2025-01-01"); err != nil {
		t.Fatalf("GetMetricaByDate() error = %v", err)
	}
	if ss.Logins != 1 {
		t.Errorf("logins = %d across two METRICA calls, want 1", ss.Logins)
	}
	if len(ss.CookiesSeen) != 2 {
		t.Fatalf("endpoint calls = %d, want 2", len(ss.CookiesSeen))
	}
	for i, got := range ss.CookiesSeen {
		if got == "" {
			t.Errorf("METRICA call %d carried no session cookie", i+1)
		}
	}
}
