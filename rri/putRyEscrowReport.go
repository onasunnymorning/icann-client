package rri

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

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

	res, status, err := c.doReportPut(req)
	if err != nil {
		return nil, err
	}

	return &SubmitResult{
		TLD:        cfg.TLD,
		ID:         id,
		URL:        req.URL.String(),
		HTTPStatus: status,
		ResultCode: res.Result.Code,
		Message:    res.Result.Msg,
	}, nil
}
