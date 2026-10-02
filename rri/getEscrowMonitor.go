package rri

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// Namespaces of the two monitoring documents added in draft -27.
const (
	NamespaceRdeReports       = "urn:ietf:params:xml:ns:rdeReports-1.0"
	NamespaceRdeNotifications = "urn:ietf:params:xml:ns:rdeNotifications-1.0"
	NamespaceRdeNotification  = "urn:ietf:params:xml:ns:rdeNotification-1.0"
)

// getEscrowMonitor performs the GET shared by both monitoring interfaces and
// returns the body to decode, or nil when ICANN holds nothing for that date.
//
// The draft defines HTTP 404 as exactly that answer, so it is not a failure.
// The same status also comes back for an unknown route or a refused credential,
// with an HTML or JSON error body, and those must stay errors: otherwise a
// typo'd TLD would read as "nothing received". A result envelope is likewise
// never an empty answer, so it stays the *ResultError doGet built.
//
// No Accept header, for the reason given in GetConformanceVersion.
func (c *Client) getEscrowMonitor(ctx context.Context, kind string, date time.Time) (*http.Request, []byte, error) {
	path := fmt.Sprintf("/info/report/%s/%s/%s", kind, c.Config().TLD, date.Format("2006-01-02"))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	raw, status, err := c.doGet(req)
	if status == http.StatusNotFound && !IsRejected(err) {
		var httpErr *base.HTTPError
		if errors.As(err, &httpErr) && looksLikeErrorPage(raw) {
			return req, nil, err
		}
		return req, nil, nil
	}
	if err != nil {
		return req, nil, err
	}
	return req, raw, nil
}

// looksLikeErrorPage reports whether a 404 body is a server or gateway error
// page rather than ICANN's empty "nothing received" answer.
func looksLikeErrorPage(raw []byte) bool {
	l := bytes.ToLower(raw)
	for _, marker := range []string{"<html", "unauthorized", "error"} {
		if bytes.Contains(l, []byte(marker)) {
			return true
		}
	}
	return false
}
