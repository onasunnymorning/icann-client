package rri

import (
	"fmt"
	"io"
	"net/http"

	base "github.com/onasunnymorning/icann-client/client"
)

// doGet performs a read request and returns the response body for the caller
// to decode. It is the read-side counterpart to doReportPut and classifies
// failures in the same deliberate order, because ICANN answers a rejected read
// with the same IIRDEA result envelope it uses for a rejected submission.
//
// It returns the body rather than decoding into a destination because ICANN's
// read endpoints do not agree on a format: the reporting summary comes back as
// JSON, while the draft specifies XML. Each endpoint therefore decodes what it
// actually receives, and the classification that must not drift lives here.
//
// On failure the error is either a *ResultError (ICANN understood the request
// and refused it, which it may do with HTTP 200) or a *client.HTTPError. The
// two are alternatives and are never nested.
//
// The status code is returned alongside the error so a caller can treat a
// particular status as meaningful rather than as a failure —
// GetConformanceVersion does exactly that with 404.
func (c *Client) doGet(req *http.Request) ([]byte, int, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Read the body whatever the status: it carries the result code on a
	// rejection, and draining it lets the connection be reused. RRI rate-limits
	// on authentication, so reuse is a correctness requirement rather than
	// good manners.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	// A result envelope means ICANN answered with a code rather than the
	// document. Check it first: a rejection commonly arrives with HTTP 200, so
	// the status alone must never be read as success.
	if res, parseErr := parseIIRDEAResponse(raw); parseErr == nil && res.Result.Code != ResultSuccess {
		return raw, resp.StatusCode, &ResultError{
			Code:       res.Result.Code,
			Msg:        res.Result.Msg,
			HTTPStatus: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return raw, resp.StatusCode, &base.HTTPError{
			StatusCode: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
			Body:       string(raw),
		}
	}
	return raw, resp.StatusCode, nil
}

// decodeError reports a body that arrived with HTTP 200 but did not match the
// shape this client expects. It is deliberately not an *client.HTTPError:
// "http error: 200" describes a successful request and tells the reader
// nothing, whereas the cause here is the payload.
func decodeError(req *http.Request, what string, raw []byte, err error) error {
	const excerpt = 2048
	body := raw
	if len(body) > excerpt {
		body = body[:excerpt]
	}
	return fmt.Errorf("%s %s returned HTTP 200 but the body is not %s: %w\nbody: %s",
		req.Method, req.URL, what, err, body)
}
