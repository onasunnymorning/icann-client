package rri

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"

	base "github.com/onasunnymorning/icann-client/client"
)

// Result codes returned by the ICANN reporting interfaces, as defined by
// draft-lozano-icann-registry-interfaces. The registry escrow report (Section
// 2.3) and the two Specification 3 monthly reports (Section 3) share one
// envelope and overlapping ranges: 20xx codes are common, 21xx codes are
// specific to the monthly CSV reports.
const (
	ResultSuccess               = 1000 // no errors found; the report was accepted
	ResultBadRequest            = 2001 // the request did not validate against the schema
	ResultReportExistsCutOff    = 2002 // a report for that month exists and the cut-off date has passed
	ResultNegativeValues        = 2003 // the report contains negative numeric values
	ResultDateInFuture          = 2004 // crDate or watermark is in the future, or the month has not ended
	ResultUnsupportedVersion    = 2005 // version is not supported
	ResultIDMismatch            = 2006 // the id in the report and in the URL path differ
	ResultInterfaceDisabled     = 2007 // the interface is disabled for this TLD
	ResultDateBeforeTLDCreation = 2008 // crDate or watermark predates the TLD
	ResultTLDMismatch           = 2202 // the tld in the header and in the URL path differ
	ResultUnexpectedDIFF        = 2205 // a differential report arrived where a full one was expected
	ResultDuplicateDomainCount  = 2206 // both csvDomain and rdeDomain counts were provided
	ResultMissingTLD            = 2209 // the header is missing its required tld element
	ResultRCDNMismatch          = 2210 // a count element carries an unexpected rcdn attribute
	ResultDuplicateCount        = 2211 // several count elements share uri, rcdn and registrarId
	ResultInvalidLabel          = 2212 // an invalid label or domain name syntax was found

	// Codes specific to the Specification 3 monthly CSV reports (Section 3).
	ResultIncorrectTotals        = 2101 // the totals line does not match the sum of the data lines
	ResultRegistrarNotAccredited = 2102 // a registrar in the report is not ICANN-accredited
	ResultTotalsLineNotEmpty     = 2103 // the second field of the totals line is not empty
	ResultNotUTF8                = 2105 // the report is not encoded in UTF-8 (US-ASCII is accepted)
	ResultInvalidDateInURL       = 2111 // the date in the URL is not a valid YYYY-MM month
)

// ResultHint returns a short, actionable note for the result codes a backfill
// is most likely to hit, or the empty string when there is nothing useful to
// add beyond ICANN's own message.
func ResultHint(code int) string {
	switch code {
	case ResultReportExistsCutOff:
		return "the cut-off date for this month has passed, so ICANN will not accept a replacement; contact ICANN Global Support to have it reopened"
	case ResultIncorrectTotals:
		return "the totals line does not match the sum of the data lines; run with --dry-run to see which column is off"
	case ResultTotalsLineNotEmpty:
		return "the second field of the totals line must be empty"
	case ResultNotUTF8:
		return "re-encode the file as UTF-8 before resubmitting"
	case ResultInvalidDateInURL:
		return "the month must be YYYY-MM; pass --month to override what was read from the filename"
	case ResultInterfaceDisabled:
		return "the interface is disabled for this TLD; this is the one rejection worth retrying later"
	case ResultDateInFuture:
		return "ICANN accepts a month only once it has ended"
	case ResultRegistrarNotAccredited:
		return "one of the registrars in the report is not ICANN-accredited for this TLD"
	default:
		return ""
	}
}

// iirdeaResponse is the result envelope ICANN returns from the reporting
// interfaces. It is sent with both HTTP 200 and HTTP 400 responses.
type iirdeaResponse struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:iirdea-1.0 response"`
	Result  struct {
		Code int    `xml:"code,attr"`
		Msg  string `xml:"urn:ietf:params:xml:ns:iirdea-1.0 msg"`
	} `xml:"urn:ietf:params:xml:ns:iirdea-1.0 result"`
}

// parseIIRDEAResponse decodes a result envelope. It is deliberately strict: a
// body that is empty, is not XML, or is rooted at some other element is an
// error rather than an assumed success.
func parseIIRDEAResponse(b []byte) (*iirdeaResponse, error) {
	if len(b) == 0 {
		return nil, errors.New("empty response body")
	}
	var r iirdeaResponse
	if err := newXMLDecoder(b).Decode(&r); err != nil {
		return nil, fmt.Errorf("parsing ICANN response: %w", err)
	}
	if r.Result.Code == 0 {
		return nil, errors.New("ICANN response carries no result code")
	}
	return &r, nil
}

// ResultError is a business-level rejection: ICANN understood the request and
// answered with a result code other than 1000.
//
// It is distinct from *client.HTTPError, which reports a transport- or
// protocol-level failure where no result code could be read. The two are
// alternatives and are never nested, so callers branch on the result code
// rather than on the HTTP status. Note in particular that a rejection commonly
// arrives with HTTP 200.
type ResultError struct {
	Code       int
	Msg        string
	HTTPStatus int
	Method     string
	URL        string
}

// Error implements the error interface.
func (e *ResultError) Error() string {
	return fmt.Sprintf("rejected by ICANN: result code %d (http %d) %s %s - %s",
		e.Code, e.HTTPStatus, e.Method, e.URL, e.Msg)
}

// Retryable reports whether re-submitting the identical report could plausibly
// succeed later. Schema and identity errors are permanent; an interface that is
// temporarily disabled for the TLD is not.
func (e *ResultError) Retryable() bool {
	return e.Code == ResultInterfaceDisabled
}

// ResultCodeOf returns the ICANN result code carried by err, and reports
// whether err (or an error it wraps) was a *ResultError.
func ResultCodeOf(err error) (int, bool) {
	var re *ResultError
	if errors.As(err, &re) {
		return re.Code, true
	}
	return 0, false
}

// IsRejected reports whether err is a business-level rejection carrying a
// result code, as opposed to a transport or HTTP-level failure.
func IsRejected(err error) bool {
	_, ok := ResultCodeOf(err)
	return ok
}

// IsRetryable reports whether err is worth retrying: a server-side HTTP
// failure, a retryable rejection, or a timed-out request. Schema and identity
// rejections are not retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var re *ResultError
	if errors.As(err, &re) {
		return re.Retryable()
	}
	var he *base.HTTPError
	if errors.As(err, &he) {
		return he.StatusCode >= 500
	}
	return errors.Is(err, context.DeadlineExceeded)
}
