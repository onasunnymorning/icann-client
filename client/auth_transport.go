package client

import (
	"net/http"
)

// basicAuthTransport injects HTTP Basic Authorization header for all requests.
type basicAuthTransport struct {
	username string
	password string
	base     http.RoundTripper
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the caller's request
	r := req.Clone(req.Context())
	r.SetBasicAuth(t.username, t.password)
	return t.base.RoundTrip(r)
}
