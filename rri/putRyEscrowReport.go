package rri

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	base "github.com/onasunnymorning/icann-client/client"
)

// maxResponseBody caps how much of a response we read. Result envelopes are a
// few hundred bytes; the cap only guards against a pathological error page.
const maxResponseBody = 1 << 20

// SubmitResult describes an accepted registry escrow report submission.
type SubmitResult struct {
	TLD        string `json:"tld"`
	ID         string `json:"id"`
	URL        string `json:"url"`
	HTTPStatus int    `json:"httpStatus"`
	ResultCode int    `json:"resultCode"`
	Message    string `json:"message,omitempty"`
}

// SubmitRyEscrowReport submits an RDE (registry escrow) report for the client's
// configured TLD, per draft-lozano-icann-registry-interfaces Section 2.3.
//
// body must be the exact bytes of the report document. They are transmitted
// verbatim and are never re-serialized, so that any hash or signature over the
// document remains valid. id must equal the <rdeReport:id> element inside the
// document and the id attribute of the corresponding escrow <deposit>.
//
// A nil error means ICANN answered with result code 1000. HTTP 200 alone does
// not indicate acceptance: a rejection is returned as a *ResultError carrying
// the result code, whether the status was 200 or 400. Transport failures and
// responses with no readable result code are returned as *client.HTTPError.
//
// Submitting a report whose id was already accepted overwrites the previous
// submission, so the call is safe to repeat.
func (c *Client) SubmitRyEscrowReport(ctx context.Context, id string, body []byte) (*SubmitResult, error) {
	if id == "" {
		return nil, errors.New("report id is required")
	}
	if len(body) == 0 {
		return nil, errors.New("report body is empty")
	}
	cfg := c.Config()

	// Note there is no /info/ prefix here. That prefix belongs to the status
	// endpoint (Section 2.3.1); submissions go to /report/... .
	path := fmt.Sprintf("/report/registry-escrow-report/%s/%s",
		url.PathEscape(cfg.TLD), url.PathEscape(id))

	// A bytes.Reader gives the request an accurate ContentLength and a GetBody,
	// so a redirect or transport retry can replay the body.
	req, err := c.NewRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml") // required by Section 2.3
	req.Header.Set("Accept", "text/xml")

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Always read the body, whatever the status: it carries the result code,
	// and draining it lets the connection be reused for the next report. ICANN
	// rate-limits on authentication, so connection reuse is load-bearing.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
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
		// No result code to report on, so surface the raw HTTP failure. We
		// never infer success from a 2xx with an unreadable body.
		return nil, httpErr()
	}
	if res.Result.Code != ResultSuccess {
		return nil, &ResultError{
			Code:       res.Result.Code,
			Msg:        res.Result.Msg,
			HTTPStatus: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
		}
	}
	if resp.StatusCode != http.StatusOK {
		// Result code 1000 on a non-200 status contradicts the specification;
		// refuse to report it as an acceptance.
		return nil, httpErr()
	}

	return &SubmitResult{
		TLD:        cfg.TLD,
		ID:         id,
		URL:        req.URL.String(),
		HTTPStatus: resp.StatusCode,
		ResultCode: res.Result.Code,
		Message:    res.Result.Msg,
	}, nil
}
