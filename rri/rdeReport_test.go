package rri

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testNow pins the clock so date-dependent fixtures never age.
var testNow = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

func TestParseRyEscrowReport(t *testing.T) {
	want := ReportMeta{
		ID:        "example-20250101-full",
		Version:   1,
		CrDate:    time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC),
		Kind:      KindFull,
		Watermark: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TLD:       "example",
		Counts: []ObjectCount{
			{URI: "urn:ietf:params:xml:ns:rdeDomain-1.0", Count: 1250},
			{URI: "urn:ietf:params:xml:ns:rdeHost-1.0", RCDN: "example", Count: 310},
		},
	}

	// The prefixed, default-namespace and BOM-prefixed documents are the same
	// report and must all decode identically.
	for _, name := range []string{"report-full-prefixed.xml", "report-full-defaultns.xml", "report-bom.xml"} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseRyEscrowReport(readFixture(t, name))
			if err != nil {
				t.Fatalf("ParseRyEscrowReport() error = %v", err)
			}
			if got.ID != want.ID || got.Version != want.Version || got.Kind != want.Kind || got.TLD != want.TLD {
				t.Errorf("scalar fields = %+v, want %+v", *got, want)
			}
			if !got.CrDate.Equal(want.CrDate) {
				t.Errorf("CrDate = %v, want %v", got.CrDate, want.CrDate)
			}
			if !got.Watermark.Equal(want.Watermark) {
				t.Errorf("Watermark = %v, want %v", got.Watermark, want.Watermark)
			}
			if len(got.Counts) != len(want.Counts) {
				t.Fatalf("Counts = %+v, want %+v", got.Counts, want.Counts)
			}
			for i := range want.Counts {
				if got.Counts[i] != want.Counts[i] {
					t.Errorf("Counts[%d] = %+v, want %+v", i, got.Counts[i], want.Counts[i])
				}
			}
		})
	}
}

func TestParseRyEscrowReportErrors(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		raw     []byte
		wantErr error
	}{
		{name: "missing id", fixture: "report-no-id.xml", wantErr: ErrReportIDMissing},
		{name: "missing tld", fixture: "report-no-tld.xml", wantErr: ErrReportTLDMissing},
		{name: "not xml", raw: []byte("this is not xml")},
		{name: "empty", raw: []byte("")},
		{name: "wrong root element", raw: []byte(`<foo xmlns="urn:ietf:params:xml:ns:rdeReport-1.0"/>`)},
		{name: "unsupported encoding", raw: []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><report xmlns="urn:ietf:params:xml:ns:rdeReport-1.0"/>`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.raw
			if tt.fixture != "" {
				b = readFixture(t, tt.fixture)
			}
			_, err := ParseRyEscrowReport(b)
			if err == nil {
				t.Fatal("ParseRyEscrowReport() expected an error, got nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestReportMetaValidate(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		opts    ValidationOptions
		wantOK  bool
		// wantCode is the ICANN result code the message should reference.
		wantCode int
	}{
		{
			name:    "valid",
			fixture: "report-full-prefixed.xml",
			opts:    ValidationOptions{TLD: "example", ID: "example-20250101-full", Now: testNow},
			wantOK:  true,
		},
		{
			name:    "tld compared case-insensitively",
			fixture: "report-full-prefixed.xml",
			opts:    ValidationOptions{TLD: "EXAMPLE", Now: testNow},
			wantOK:  true,
		},
		{
			name:     "tld mismatch",
			fixture:  "report-full-prefixed.xml",
			opts:     ValidationOptions{TLD: "other", Now: testNow},
			wantCode: ResultTLDMismatch,
		},
		{
			name:     "id mismatch",
			fixture:  "report-full-prefixed.xml",
			opts:     ValidationOptions{ID: "something-else", Now: testNow},
			wantCode: ResultIDMismatch,
		},
		{
			name:     "watermark in the future",
			fixture:  "report-future-watermark.xml",
			opts:     ValidationOptions{TLD: "example", Now: testNow},
			wantCode: ResultDateInFuture,
		},
		{
			name:     "duplicate count elements",
			fixture:  "report-duplicate-count.xml",
			opts:     ValidationOptions{TLD: "example", Now: testNow},
			wantCode: ResultDuplicateCount,
		},
		{
			name:     "both domain models counted",
			fixture:  "report-both-domain-models.xml",
			opts:     ValidationOptions{TLD: "example", Now: testNow},
			wantCode: ResultDuplicateDomainCount,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseRyEscrowReport(readFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("ParseRyEscrowReport() error = %v", err)
			}
			err = m.Validate(tt.opts)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if want := strconv.Itoa(tt.wantCode); !strings.Contains(err.Error(), want) {
				t.Errorf("Validate() = %v, want a message mentioning result code %d", err, tt.wantCode)
			}
		})
	}
}
