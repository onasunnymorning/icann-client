// Package mosapi provides a high-level client for ICANN MOSAPI endpoints.
//
// It composes the shared HTTP/auth client from the parent module (package client)
// so that both MOSAPI and RRI packages can share credentials and transport
// configuration while exposing service-specific methods.
//
// MOSAPI is session based: credentials are accepted only at the unversioned
// /<entity>/<tld>/login endpoint, and the versioned endpoints authenticate with
// the session cookie it returns (or with a TLS client certificate). Client
// handles this transparently — it logs in on demand, reuses the session, and
// renews it once when the server reports it expired — so callers do not need to
// call Login themselves. Note that ICANN permits only one concurrent session
// per account and expires it after 15 minutes, so a client should be created
// once and reused rather than per request.
package mosapi
