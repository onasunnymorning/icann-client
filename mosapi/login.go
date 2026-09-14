package mosapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	base "github.com/onasunnymorning/icann-client/client"
)

// sessionCookieName is the name of the session cookie MOSAPI sets on a
// successful login. Its value is a 160-bit random number in Base16.
const sessionCookieName = "id"

// maxLoginBody caps how much of a login or logout response is read. Both are
// short text/plain bodies; the limit only guards against a misbehaving proxy.
const maxLoginBody = 1 << 16

// loginPath returns the unversioned login or logout endpoint. Note the absence
// of the API version: MOSAPI places session management at
// /<entity>/<tld>/login, one level above the versioned endpoints, and the
// session cookie it returns is scoped to /<entity>/<tld> and all sub-paths.
func (c *Client) loginPath(action string) string {
	cfg := c.Config()
	return fmt.Sprintf("/%s/%s/%s", cfg.Entity, cfg.TLD, action)
}

// sessionURL is the URL the session cookie is scoped to. It is the address the
// cookie jar is queried against, not an endpoint that is ever requested.
func (c *Client) sessionURL() *url.URL {
	cfg := c.Config()
	u := c.BaseURL()
	u.Path = fmt.Sprintf("/%s/%s/", cfg.Entity, cfg.TLD)
	u.RawQuery = ""
	return u
}

// HasSession reports whether the client currently holds a MOSAPI session
// cookie. It does not prove the session is still valid server side: sessions
// expire after 15 minutes and may be evicted when the account logs in
// elsewhere, which surfaces as a 401 on the next request.
func (c *Client) HasSession() bool {
	jar := c.HTTPClient.Jar
	if jar == nil {
		return false
	}
	for _, ck := range jar.Cookies(c.sessionURL()) {
		if ck.Name == sessionCookieName && ck.Value != "" {
			return true
		}
	}
	return false
}

// Login authenticates against MOSAPI and stores the session cookie in the
// client's cookie jar.
//
// MOSAPI is session based: credentials are accepted only at this endpoint, and
// every other endpoint authenticates with the resulting cookie (or with a TLS
// client certificate). Callers do not normally need to call Login themselves —
// the API methods log in on demand and renew an expired session once — but it
// is exported so that a caller can establish or refresh a session explicitly.
//
// Only one concurrent session is permitted per account. Creating a new session
// terminates the account's oldest one, so a long-lived client should reuse its
// session rather than log in per request.
func (c *Client) Login(ctx context.Context) error {
	cfg := c.Config()
	req, err := c.NewRequest(ctx, http.MethodGet, c.loginPath("login"), nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("MOSAPI login: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxLoginBody))

	if resp.StatusCode != http.StatusOK {
		herr := &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: string(b)}
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("MOSAPI login failed for %s: check the username and password for this profile: %w", cfg.TLD, herr)
		}
		return fmt.Errorf("MOSAPI login: %w", herr)
	}

	// A 200 without a cookie would leave every later request unauthenticated and
	// looking like a credential problem, so it is an error here rather than later.
	if !c.HasSession() {
		return fmt.Errorf("MOSAPI login returned HTTP 200 but no %q session cookie: %s", sessionCookieName, req.URL.String())
	}
	return nil
}

// Logout terminates the current MOSAPI session and drops the cookie. It is a
// no-op when no session is held.
//
// Calling it is optional: a session expires on its own after 15 minutes, and
// logging in again terminates it. It is worth calling from a long-running
// process that is done talking to MOSAPI, so the account's single session slot
// is released immediately.
func (c *Client) Logout(ctx context.Context) error {
	if !c.HasSession() {
		return nil
	}
	req, err := c.NewRequest(ctx, http.MethodGet, c.loginPath("logout"), nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("MOSAPI logout: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxLoginBody))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MOSAPI logout: %w", &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: string(b)})
	}
	c.clearSession()
	return nil
}

// clearSession drops the session cookie by replacing the jar. net/http/cookiejar
// has no delete operation, and expiring the cookie by hand would have to guess
// the exact path the server scoped it to.
func (c *Client) clearSession() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return
	}
	c.HTTPClient.Jar = jar
}

// usesSession reports whether this client authenticates with a session cookie.
// TLS client certificate authentication is accepted directly by the versioned
// endpoints and needs no login.
func (c *Client) usesSession() bool {
	return c.Config().AuthType == base.AUTH_TYPE_BASIC
}

// ensureSession logs in if the client authenticates by session and does not
// already hold one.
func (c *Client) ensureSession(ctx context.Context) error {
	if !c.usesSession() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.HasSession() {
		return nil
	}
	return c.Login(ctx)
}

// renewSession logs in again, replacing whatever session the client held. The
// lock is held across the login so that concurrent callers reacting to the same
// expiry do not each start a session — which would evict one another, since
// MOSAPI permits only one per account.
func (c *Client) renewSession(ctx context.Context, staleCookie string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur := c.sessionValue(); cur != "" && cur != staleCookie {
		// Another caller already renewed it; use theirs.
		return nil
	}
	return c.Login(ctx)
}

// sessionValue returns the current session cookie value, or "" when none is held.
func (c *Client) sessionValue() string {
	jar := c.HTTPClient.Jar
	if jar == nil {
		return ""
	}
	for _, ck := range jar.Cookies(c.sessionURL()) {
		if ck.Name == sessionCookieName {
			return ck.Value
		}
	}
	return ""
}

// do executes a MOSAPI request, establishing a session first when one is
// needed and renewing it once if the server rejects it.
//
// The retry exists because MOSAPI sessions expire after 15 minutes and are
// evicted when the same account logs in elsewhere, so a 401 is a routine,
// recoverable condition rather than a credential error. It is attempted at most
// once: a 401 from a freshly minted session is a real authentication failure and
// is returned to the caller.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}

	// Capture the session the request is about to use, so a concurrent renewal
	// can be told apart from an expiry this call has to handle.
	used := c.sessionValue()

	retry, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized || !c.usesSession() {
		return resp, nil
	}

	// Drain and close before reusing the connection for the login.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxLoginBody))
	resp.Body.Close()

	if err := c.renewSession(ctx, used); err != nil {
		return nil, err
	}
	return c.Do(retry)
}

// cloneRequest copies a request so it can be re-issued after a session renewal.
// A request whose body cannot be replayed is not cloned; the caller then gets
// the original 401 rather than a silently truncated retry.
func cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("cannot retry %s %s after a session renewal: request body is not replayable", req.Method, req.URL)
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}
