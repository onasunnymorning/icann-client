package rri

import (
	"fmt"
	"io"
	"net/http"

	base "github.com/onasunnymorning/icann-client/client"
)

// doXMLGet performs a read request and decodes the XML document it returns
// into out. It is the read-side counterpart to doReportPut, and classifies
// outcomes in the same deliberate order, because ICANN answers a rejected read
// with the same IIRDEA result envelope it uses for a rejected submission.
//
// On failure it returns either a *ResultError (ICANN understood the request
// and refused it, which it may do with HTTP 200) or a *client.HTTPError (no
// result code could be read). The two are alternatives and are never nested.
//
// The status code is returned alongside the error so that a caller can treat a
// particular status as meaningful rather than as a failure — GetConformanceVersion
// does exactly that with 404.
func (c *Client) doXMLGet(req *http.Request, out any) (int, error) {
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Read the body whatever the status: it carries the result code on a
	// rejection, and draining it lets the connection be reused. RRI rate-limits
	// on authentication, so reuse is a correctness requirement rather than
	// good manners.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	httpErr := func() error {
		return &base.HTTPError{
			StatusCode: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
			Body:       string(raw),
		}
	}

	// A result envelope means ICANN answered with a code rather than the
	// document. Check it first: a rejection commonly arrives with HTTP 200, so
	// the status alone must never be read as success.
	if res, parseErr := parseIIRDEAResponse(raw); parseErr == nil && res.Result.Code != ResultSuccess {
		return resp.StatusCode, &ResultError{
			Code:       res.Result.Code,
			Msg:        res.Result.Msg,
			HTTPStatus: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, httpErr()
	}
	if err := newXMLDecoder(raw).Decode(out); err != nil {
		// Never infer an empty result from a body that would not decode.
		return resp.StatusCode, httpErr()
	}
	return resp.StatusCode, nil
}
