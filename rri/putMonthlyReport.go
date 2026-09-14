package rri

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// monthPattern is the YYYY-MM form required in the URL by Section 3. Checking
// it locally pre-empts result code 2111 without spending a request.
var monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// MonthlySubmitResult describes an accepted monthly report submission.
type MonthlySubmitResult struct {
	TLD        string     `json:"tld"`
	Type       ReportType `json:"type"`
	Month      string     `json:"month"`
	URL        string     `json:"url"`
	HTTPStatus int        `json:"httpStatus"`
	ResultCode int        `json:"resultCode"`
	Message    string     `json:"message,omitempty"`
}

// monthlyReportPath returns the URL path segment for a monthly report type.
// The two spellings live here alone so they cannot drift between the
// submission and status endpoints.
func monthlyReportPath(typ ReportType) (string, error) {
	switch typ {
	case ReportTransactions:
		return "registrar-transactions", nil
	case ReportActivity:
		return "registry-functions-activity", nil
	default:
		return "", fmt.Errorf("unknown report type %q; expected %q or %q", typ, ReportTransactions, ReportActivity)
	}
}

// SubmitMonthlyReport submits a Specification 3 monthly report for the client's
// configured TLD and the given YYYY-MM month, per
// draft-lozano-icann-registry-interfaces Section 3.
//
// body must be the exact bytes of the CSV file. They are transmitted verbatim
// and are never re-serialized, so a byte-order mark or an unusual line ending
// reaches ICANN exactly as it sits on disk.
//
// A nil error means ICANN answered with result code 1000. HTTP 200 alone does
// not indicate acceptance: a rejection is returned as a *ResultError carrying
// the result code, whether the status was 200 or 400. Transport failures and
// responses with no readable result code are returned as *client.HTTPError.
//
// Before the month's cut-off date a report may be replaced as many times as
// needed, so the call is safe to repeat. After the cut-off ICANN rejects a
// replacement with ResultReportExistsCutOff.
func (c *Client) SubmitMonthlyReport(ctx context.Context, typ ReportType, month string, body []byte) (*MonthlySubmitResult, error) {
	seg, err := monthlyReportPath(typ)
	if err != nil {
		return nil, err
	}
	if !monthPattern.MatchString(month) {
		return nil, fmt.Errorf("month %q must be in YYYY-MM form", month)
	}
	if len(body) == 0 {
		return nil, errors.New("report body is empty")
	}
	cfg := c.Config()

	// No /info/ prefix: that belongs to the status endpoint (Section 3.3).
	path := fmt.Sprintf("/report/%s/%s/%s", seg, url.PathEscape(cfg.TLD), url.PathEscape(month))

	// A bytes.Reader gives the request an accurate ContentLength and a GetBody,
	// so a redirect or transport retry can replay the body.
	req, err := c.NewRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/csv") // required by Section 3
	req.Header.Set("Accept", "text/xml")

	res, status, err := c.doReportPut(req)
	if err != nil {
		return nil, err
	}

	return &MonthlySubmitResult{
		TLD:        cfg.TLD,
		Type:       typ,
		Month:      month,
		URL:        req.URL.String(),
		HTTPStatus: status,
		ResultCode: res.Result.Code,
		Message:    res.Result.Msg,
	}, nil
}

// GetMonthlyReportStatus reports whether ICANN holds a valid monthly report of
// the given type for the client's TLD and the given YYYY-MM month, per
// draft-lozano-icann-registry-interfaces Section 3.3.
func (c *Client) GetMonthlyReportStatus(ctx context.Context, typ ReportType, month string) (*ReportStatus, error) {
	seg, err := monthlyReportPath(typ)
	if err != nil {
		return nil, err
	}
	if !monthPattern.MatchString(month) {
		return nil, fmt.Errorf("month %q must be in YYYY-MM form", month)
	}
	cfg := c.Config()

	path := fmt.Sprintf("/info/report/%s/%s/%s", seg, url.PathEscape(cfg.TLD), url.PathEscape(month))
	req, err := c.NewRequest(ctx, http.MethodHead, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// ReportStatus.Date carries the first day of the month, so a monthly status
	// renders the same way an escrow one does rather than showing a zero time.
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, err
	}
	rs := &ReportStatus{Type: string(typ), TLD: cfg.TLD, Date: first}
	switch resp.StatusCode {
	case http.StatusOK:
		rs.Status = RY_RDEReport_RECEIVED
		return rs, nil
	case http.StatusNotFound:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		body := string(b)
		// A 404 carrying an HTML error page or an authorization complaint is a
		// failed request, not a genuine "no report for this month".
		if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || strings.Contains(strings.ToLower(body), "<html") {
			return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: body}
		}
		if strings.Contains(strings.ToLower(body), "unauthorized") || strings.Contains(strings.ToLower(body), "error") {
			return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: body}
		}
		rs.Status = RY_RDEReport_PENDING
		return rs, nil
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String(), Body: string(b)}
	}
}
