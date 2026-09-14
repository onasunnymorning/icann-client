package rri

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strconv"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

// newTestRRI returns an RRI client pointed at a test server running handler.
func newTestRRI(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	c, err := New(base.Config{
		TLD:      "example",
		AuthType: base.AUTH_TYPE_BASIC,
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.WithBaseURL(srv.URL); err != nil {
		t.Fatalf("WithBaseURL() error = %v", err)
	}
	return c
}

// respond writes an IIRDEA result envelope with the given status and code.
func respond(w http.ResponseWriter, status, code int) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0">
  <result code="` + strconv.Itoa(code) + `"><msg>test message</msg></result>
</response>`))
}

// TestSubmitRyEscrowReportRequestShape pins the wire format: the submission
// path has no /info/ prefix, the content type is mandated by the spec, and the
// body must reach the server byte for byte.
func TestSubmitRyEscrowReportRequestShape(t *testing.T) {
	fixture := readFixture(t, "report-bom.xml") // BOM included on purpose

	var (
		gotMethod, gotPath, gotContentType string
		gotUser, gotPass                   string
		gotHasAuth                         bool
		gotLength                          int64
		gotBody                            []byte
	)
	cli := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotUser, gotPass, gotHasAuth = r.BasicAuth()
		gotContentType = r.Header.Get("Content-Type")
		gotLength = r.ContentLength
		gotBody, _ = readAll(r)
		respond(w, http.StatusOK, ResultSuccess)
	})

	res, err := cli.SubmitRyEscrowReport(context.Background(), "example-20250101-full", fixture)
	if err != nil {
		t.Fatalf("SubmitRyEscrowReport() error = %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if want := "/report/registry-escrow-report/example/example-20250101-full"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if strings.Contains(gotPath, "/info/") {
		t.Errorf("path %q must not carry the /info/ prefix; that belongs to the status endpoint", gotPath)
	}
	if gotContentType != "text/xml" {
		t.Errorf("Content-Type = %q, want text/xml", gotContentType)
	}
	// Credentials must ride along on the PUT itself: there is no login step, so
	// a missing header here would surface only as a 401 from ICANN.
	if !gotHasAuth {
		t.Error("no Authorization header on the PUT")
	}
	if gotUser != "user" || gotPass != "pass" {
		t.Errorf("basic auth = %q/%q, want user/pass", gotUser, gotPass)
	}
	if gotLength != int64(len(fixture)) {
		t.Errorf("ContentLength = %d, want %d", gotLength, len(fixture))
	}
	if !bytes.Equal(gotBody, fixture) {
		t.Error("request body differs from the file on disk; the report must be submitted verbatim, never re-serialized")
	}
	if res.ResultCode != ResultSuccess || res.ID != "example-20250101-full" || res.TLD != "example" {
		t.Errorf("result = %+v", res)
	}
}

func readAll(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func TestSubmitRyEscrowReport(t *testing.T) {
	tests := []struct {
		name string
		// handler overrides the default envelope responder when set.
		handler    func(w http.ResponseWriter, r *http.Request)
		status     int
		code       int
		wantOK     bool
		wantCode   int
		wantHTTP   int
		wantRetry  bool
		wantReject bool
	}{
		{name: "accepted", status: http.StatusOK, code: ResultSuccess, wantOK: true},
		{
			// The most important case: a 2xx status is not an acceptance.
			name: "rejected with http 200", status: http.StatusOK, code: ResultBadRequest,
			wantCode: ResultBadRequest, wantHTTP: http.StatusOK, wantReject: true,
		},
		{
			name: "rejected with http 400", status: http.StatusBadRequest, code: ResultIDMismatch,
			wantCode: ResultIDMismatch, wantHTTP: http.StatusBadRequest, wantReject: true,
		},
		{
			name: "interface disabled is retryable", status: http.StatusBadRequest, code: ResultInterfaceDisabled,
			wantCode: ResultInterfaceDisabled, wantHTTP: http.StatusBadRequest, wantReject: true, wantRetry: true,
		},
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantHTTP: http.StatusUnauthorized,
		},
		{
			name: "server failure is retryable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("<html><body>500</body></html>"))
			},
			wantHTTP: http.StatusInternalServerError, wantRetry: true,
		},
		{
			name: "unparseable body is never a silent success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not xml at all"))
			},
			wantHTTP: http.StatusOK,
		},
		{
			name: "wrong root element",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<foo xmlns="urn:ietf:params:xml:ns:iirdea-1.0"/>`))
			},
			wantHTTP: http.StatusOK,
		},
		{
			name: "result code 1000 on a non-200 status is refused",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				respond(w, http.StatusAccepted, ResultSuccess)
			},
			wantHTTP: http.StatusAccepted,
		},
	}

	body := []byte("<report/>")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.handler
			if h == nil {
				status, code := tt.status, tt.code
				h = func(w http.ResponseWriter, _ *http.Request) { respond(w, status, code) }
			}
			cli := newTestRRI(t, h)

			res, err := cli.SubmitRyEscrowReport(context.Background(), "an-id", body)

			if tt.wantOK {
				if err != nil {
					t.Fatalf("SubmitRyEscrowReport() error = %v, want nil", err)
				}
				if res.ResultCode != ResultSuccess {
					t.Errorf("ResultCode = %d, want %d", res.ResultCode, ResultSuccess)
				}
				return
			}
			if err == nil {
				t.Fatalf("SubmitRyEscrowReport() = %+v, want an error", res)
			}
			if res != nil {
				t.Errorf("result = %+v, want nil alongside an error", res)
			}

			if tt.wantReject {
				code, ok := ResultCodeOf(err)
				if !ok {
					t.Fatalf("error = %v, want a *ResultError", err)
				}
				if code != tt.wantCode {
					t.Errorf("result code = %d, want %d", code, tt.wantCode)
				}
				var he *base.HTTPError
				if errors.As(err, &he) {
					t.Error("a business rejection must not also be an *HTTPError; the two are alternatives")
				}
			} else {
				if IsRejected(err) {
					t.Errorf("error = %v, want an *HTTPError rather than a rejection", err)
				}
				var he *base.HTTPError
				if !errors.As(err, &he) {
					t.Fatalf("error = %v, want a *client.HTTPError", err)
				}
				if he.StatusCode != tt.wantHTTP {
					t.Errorf("status = %d, want %d", he.StatusCode, tt.wantHTTP)
				}
			}
			if got := IsRetryable(err); got != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

func TestSubmitRyEscrowReportRejectsEmptyInput(t *testing.T) {
	cli := newTestRRI(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request should be issued for invalid input")
	})
	if _, err := cli.SubmitRyEscrowReport(context.Background(), "", []byte("<report/>")); err == nil {
		t.Error("expected an error for an empty id")
	}
	if _, err := cli.SubmitRyEscrowReport(context.Background(), "an-id", nil); err == nil {
		t.Error("expected an error for an empty body")
	}
}

func TestSubmitRyEscrowReportTransportError(t *testing.T) {
	cli := newTestRRI(t, func(w http.ResponseWriter, _ *http.Request) { respond(w, http.StatusOK, ResultSuccess) })
	want := errors.New("dial failed")
	cli.HTTPClient = &http.Client{Transport: &mockRoundTripper{err: want}}

	if _, err := cli.SubmitRyEscrowReport(context.Background(), "an-id", []byte("<report/>")); !errors.Is(err, want) {
		t.Errorf("error = %v, want %v", err, want)
	}
}

// TestSubmitRyEscrowReportReusesConnection guards the property that makes a
// multi-day backfill safe: ICANN rate-limits on authentication, so a batch must
// travel over a single keep-alive connection rather than re-authenticating per
// report. A response body left undrained, or a client rebuilt per file, breaks
// this.
func TestSubmitRyEscrowReportReusesConnection(t *testing.T) {
	var requests int
	cli := newTestRRI(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		respond(w, http.StatusOK, ResultSuccess)
	})

	var reused []bool
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = append(reused, info.Reused) },
	})

	for i := 0; i < 3; i++ {
		if _, err := cli.SubmitRyEscrowReport(ctx, "an-id", []byte("<report/>")); err != nil {
			t.Fatalf("submission %d: %v", i+1, err)
		}
	}

	if requests != 3 {
		t.Errorf("server saw %d requests, want 3", requests)
	}
	if len(reused) != 3 {
		t.Fatalf("got %d connections, want 3 GotConn events", len(reused))
	}
	if reused[0] {
		t.Error("first request unexpectedly reused a connection")
	}
	if !reused[1] || !reused[2] {
		t.Errorf("connection reuse = %v, want the 2nd and 3rd requests to reuse the first connection", reused)
	}
}
