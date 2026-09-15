package rri

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

func TestGetConformanceVersion(t *testing.T) {
	var gotMethod, gotPath, gotAccept string
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAccept = r.Method, r.URL.Path, r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/xml")
		w.Write(readFixture(t, "conformance-version.xml"))
	})

	got, err := c.GetConformanceVersion(t.Context())
	if err != nil {
		t.Fatalf("GetConformanceVersion() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/info/status/conformance-version" {
		t.Errorf("request = %s %s, want GET /info/status/conformance-version", gotMethod, gotPath)
	}
	// See the note in TestGetReportingStatusRequestShape: the draft requires no
	// Accept header, and constraining one drew a 406 from production.
	if gotAccept != "" {
		t.Errorf("Accept = %q, want no Accept header", gotAccept)
	}
	want := &Conformance{Specifications: []string{
		"draft-lozano-icann-registry-interfaces-26",
		"draft-icann-registrar-interfaces-16",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetConformanceVersion() = %+v, want %+v", got, want)
	}
}

// TestGetConformanceVersionNotFound is the test that stops a later refactor
// from "fixing" the 404 into an error. The draft defines a 404 as a real
// answer: the server predates this endpoint, which pins it to two specific
// specification versions.
func TestGetConformanceVersionNotFound(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	got, err := c.GetConformanceVersion(t.Context())
	if err != nil {
		t.Fatalf("GetConformanceVersion() error = %v, want the documented 404 answer", err)
	}
	if !got.Inferred {
		t.Error("Inferred = false; a 404 answer must be distinguishable from one the server sent")
	}
	want := []string{"draft-lozano-icann-registry-interfaces-25", "draft-icann-registrar-interfaces-15"}
	if !reflect.DeepEqual(got.Specifications, want) {
		t.Errorf("Specifications = %v, want %v", got.Specifications, want)
	}
}

// TestGetConformanceVersionNotFoundDoesNotShareState guards the returned slice
// against a caller that appends to it, which would corrupt the answer given to
// every later caller in the process.
func TestGetConformanceVersionNotFoundDoesNotShareState(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	first, err := c.GetConformanceVersion(t.Context())
	if err != nil {
		t.Fatalf("GetConformanceVersion() error = %v", err)
	}
	first.Specifications[0] = "mutated"

	second, err := c.GetConformanceVersion(t.Context())
	if err != nil {
		t.Fatalf("GetConformanceVersion() error = %v", err)
	}
	if second.Specifications[0] == "mutated" {
		t.Error("the 404 answer is shared between calls; a caller mutated it for everyone")
	}
}

func TestGetConformanceVersionErrors(t *testing.T) {
	t.Run("500 is an HTTP failure", func(t *testing.T) {
		c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		got, err := c.GetConformanceVersion(t.Context())
		var he *base.HTTPError
		if !errors.As(err, &he) {
			t.Fatalf("error = %v (%T), want *client.HTTPError, got %+v", err, err, got)
		}
	})

	// A 200 whose body will not decode is not an HTTP failure. Reporting it as
	// one produced "http error: 200", which describes a successful request and
	// sends the reader looking in the wrong place.
	t.Run("200 with a body that is not the document", func(t *testing.T) {
		c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not xml at all"))
		})
		got, err := c.GetConformanceVersion(t.Context())
		if err == nil {
			t.Fatalf("error = nil, got %+v", got)
		}
		var he *base.HTTPError
		if errors.As(err, &he) {
			t.Errorf("error is a *client.HTTPError (%v); the cause is the payload, not the request", err)
		}
		for _, want := range []string{"HTTP 200", "conformance document", "not xml at all"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})
}
