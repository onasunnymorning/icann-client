package rri

import (
	"fmt"
	"io"
	"net/http"

	base "github.com/onasunnymorning/icann-client/client"
)

// maxResponseBody caps how much of a response we read. Result envelopes are a
// few hundred bytes; the cap only guards against a pathological error page.
const maxResponseBody = 1 << 20

// doReportPut performs a report submission and classifies the outcome. It is
// shared by every reporting interface, because the classification order below
// is subtle and must not be allowed to drift between them.
//
// On success it returns the parsed envelope and the HTTP status. On failure it
// returns either a *ResultError (ICANN understood the request and rejected it,
// commonly with HTTP 200) or a *client.HTTPError (no result code could be
// read). The two are alternatives and are never nested.
func (c *Client) doReportPut(req *http.Request) (*iirdeaResponse, int, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Always read the body, whatever the status: it carries the result code,
	// and draining it lets the connection be reused for the next report.
	// ICANN rate-limits on authentication, so connection reuse is a
	// correctness requirement for a batch, not just good manners.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	httpErr := func() error {
		return &base.HTTPError{
			StatusCode: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
			Body:       string(raw),
		}
	}

	res, parseErr := parseIIRDEAResponse(raw)
	if parseErr != nil {
		// Never infer acceptance from a 2xx with an unreadable body.
		return nil, resp.StatusCode, httpErr()
	}
	if res.Result.Code != ResultSuccess {
		return nil, resp.StatusCode, &ResultError{
			Code:       res.Result.Code,
			Msg:        res.Result.Msg,
			HTTPStatus: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
		}
	}
	if resp.StatusCode != http.StatusOK {
		// A success code on a non-200 contradicts the spec; do not trust it.
		return nil, resp.StatusCode, httpErr()
	}
	return res, resp.StatusCode, nil
}
