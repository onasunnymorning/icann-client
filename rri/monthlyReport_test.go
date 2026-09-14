package rri

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

// jan2025 pins the clock so the future-month checks never age.
var jan2025 = time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC)

func TestParseMonthlyReportTransactions(t *testing.T) {
	m, err := ParseMonthlyReport(readFixture(t, "example-transactions-202501.csv"))
	if err != nil {
		t.Fatalf("ParseMonthlyReport: %v", err)
	}
	if m.Type != ReportTransactions {
		t.Errorf("Type = %q, want %q", m.Type, ReportTransactions)
	}
	if m.TLD != "example" {
		t.Errorf("TLD = %q, want example", m.TLD)
	}
	if m.Rows != 3 {
		t.Errorf("Rows = %d, want 3 (the totals line is not a data line)", m.Rows)
	}
	if !m.HasTotals {
		t.Error("HasTotals = false, want true")
	}
	if got := len(m.Columns); got != 7 {
		t.Errorf("len(Columns) = %d, want 7", got)
	}
	if err := m.Validate(MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025}); err != nil {
		t.Errorf("Validate on a good report: %v", err)
	}
}

func TestParseMonthlyReportActivity(t *testing.T) {
	m, err := ParseMonthlyReport(readFixture(t, "example-activity-202501.csv"))
	if err != nil {
		t.Fatalf("ParseMonthlyReport: %v", err)
	}
	if m.Type != ReportActivity {
		t.Errorf("Type = %q, want %q", m.Type, ReportActivity)
	}
	if m.Rows != 1 {
		t.Errorf("Rows = %d, want 1", m.Rows)
	}
	if m.HasTotals {
		t.Error("HasTotals = true; an activity report has no totals line")
	}
	// An activity report must not be held to the transactions totals rule.
	if err := m.Validate(MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025}); err != nil {
		t.Errorf("Validate on a good activity report: %v", err)
	}
}

func TestParseMonthlyReportBOM(t *testing.T) {
	raw := readFixture(t, "example-activity-202501-bom.csv")
	if !bytes.HasPrefix(raw, utf8BOM) {
		t.Fatal("fixture lost its BOM")
	}
	m, err := ParseMonthlyReport(raw)
	if err != nil {
		t.Fatalf("ParseMonthlyReport: %v", err)
	}
	if !m.HasBOM {
		t.Error("HasBOM = false, want true")
	}
	// The BOM must not leak into the first column name, or the tld lookup and
	// type detection both silently fail.
	if m.Columns[0] != "tld" {
		t.Errorf("Columns[0] = %q, want %q; the BOM was not stripped for parsing", m.Columns[0], "tld")
	}
	if m.Type != ReportActivity {
		t.Errorf("Type = %q, want %q", m.Type, ReportActivity)
	}
}

func TestParseMonthlyReportErrors(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		body    []byte
		code    int
	}{
		{name: "not utf-8", fixture: "example-transactions-202501-latin1.csv", code: ResultNotUTF8},
		{name: "ragged rows", fixture: "example-transactions-202501-ragged.csv", code: ResultBadRequest},
		{name: "header only", body: []byte("tld,iana-id\n"), code: ResultBadRequest},
		{name: "empty", body: []byte(""), code: ResultBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if tt.fixture != "" {
				body = readFixture(t, tt.fixture)
			}
			m, err := ParseMonthlyReport(body)
			if err == nil {
				t.Fatalf("ParseMonthlyReport succeeded, want an error; got %+v", m)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(tt.code)) {
				t.Errorf("error %q does not name result code %d", err, tt.code)
			}
		})
	}
}

func TestValidateMonthlyCodes(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		opts    MonthlyValidationOptions
		code    int
	}{
		{
			name:    "incorrect totals",
			fixture: "example-transactions-202501-badtotals.csv",
			opts:    MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025},
			code:    ResultIncorrectTotals,
		},
		{
			name:    "totals line second field not empty",
			fixture: "example-transactions-202501-totalsfield.csv",
			opts:    MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025},
			code:    ResultTotalsLineNotEmpty,
		},
		{
			name:    "negative value",
			fixture: "example-activity-202501-negative.csv",
			opts:    MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025},
			code:    ResultNegativeValues,
		},
		{
			name:    "tld mismatch",
			fixture: "example-activity-202501.csv",
			opts:    MonthlyValidationOptions{TLD: "other", Month: "2025-01", Now: jan2025},
			code:    ResultBadRequest,
		},
		{
			name:    "month in the future",
			fixture: "example-activity-202501.csv",
			opts:    MonthlyValidationOptions{TLD: "example", Month: "2025-06", Now: jan2025},
			code:    ResultDateInFuture,
		},
		{
			name:    "month not YYYY-MM",
			fixture: "example-activity-202501.csv",
			opts:    MonthlyValidationOptions{TLD: "example", Month: "2025-01-01", Now: jan2025},
			code:    ResultInvalidDateInURL,
		},
		{
			name:    "transactions report without a totals line",
			fixture: "example-activity-202501.csv",
			opts:    MonthlyValidationOptions{Type: ReportTransactions, TLD: "example", Month: "2025-01", Now: jan2025},
			code:    ResultBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseMonthlyReport(readFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("ParseMonthlyReport: %v", err)
			}
			err = m.Validate(tt.opts)
			if err == nil {
				t.Fatal("Validate succeeded, want an error")
			}
			if !strings.Contains(err.Error(), strconv.Itoa(tt.code)) {
				t.Errorf("error %q does not name result code %d", err, tt.code)
			}
		})
	}
}

// TestValidateTotalsNamesTheColumn checks that the 2101 message is specific
// enough to act on, since that is the whole point of recomputing the totals.
func TestValidateTotalsNamesTheColumn(t *testing.T) {
	m, err := ParseMonthlyReport(readFixture(t, "example-transactions-202501-badtotals.csv"))
	if err != nil {
		t.Fatalf("ParseMonthlyReport: %v", err)
	}
	err = m.Validate(MonthlyValidationOptions{TLD: "example", Month: "2025-01", Now: jan2025})
	if err == nil {
		t.Fatal("Validate succeeded, want an error")
	}
	for _, want := range []string{"net-adds-1-yr", "19", "21"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDetectReportType(t *testing.T) {
	tests := []struct {
		name    string
		columns []string
		want    ReportType
		ok      bool
	}{
		{"transactions by registrar-name", []string{"tld", "registrar-name", "total-domains"}, ReportTransactions, true},
		{"transactions by iana-id", []string{"tld", "iana-id"}, ReportTransactions, true},
		{"activity by whois", []string{"tld", "whois-43-queries"}, ReportActivity, true},
		{"activity by dns", []string{"tld", "dns-udp-queries-received"}, ReportActivity, true},
		{"case insensitive", []string{"TLD", "IANA-ID"}, ReportTransactions, true},
		{"unrecognised", []string{"tld", "something-else"}, "", false},
		{"ambiguous", []string{"tld", "iana-id", "whois-43-queries"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DetectReportType(tt.columns)
			if got != tt.want || ok != tt.ok {
				t.Errorf("DetectReportType(%v) = (%q, %v), want (%q, %v)", tt.columns, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseMonthlyFilename(t *testing.T) {
	tests := []struct {
		name  string
		month string
		typ   ReportType
	}{
		// The Specification 3 convention.
		{name: "radio-transactions-202608.csv", month: "2026-08", typ: ReportTransactions},
		{name: "radio-activity-202608.csv", month: "2026-08", typ: ReportActivity},
		{name: "/some/dir/radio-activity-202608.CSV", month: "2026-08", typ: ReportActivity},
		{name: "radio-activity-202608", month: "2026-08", typ: ReportActivity},
		{name: "RADIO-Activity-202608.csv", month: "2026-08", typ: ReportActivity},
		{name: "xn--foo-bar-transactions-202608.csv", month: "2026-08", typ: ReportTransactions},

		// What providers actually hand over: the endpoint name, a hyphenated
		// month, and no TLD prefix at all.
		{name: "registrar-transactions-2026-08.csv", month: "2026-08", typ: ReportTransactions},
		{name: "registry-functions-activity-2026-08.csv", month: "2026-08", typ: ReportActivity},
		{name: "radio-transactions-2026-08.csv", month: "2026-08", typ: ReportTransactions},
		{name: "radio_transactions_202608.csv", month: "2026-08", typ: ReportTransactions},
		{name: "transactions-202608.csv", month: "2026-08", typ: ReportTransactions},
		{name: "Registrar-Transactions-2026-08 (1).csv", month: "2026-08", typ: ReportTransactions},

		// Partial information is still worth returning: the missing half can
		// come from the CSV header or a flag.
		{name: "report-202608.csv", month: "2026-08"},
		{name: "radio-activity.csv", typ: ReportActivity},
		{name: "report.csv"},

		// Ambiguity and invalid months yield nothing rather than a guess.
		{name: "radio-transactions-activity-202608.csv", month: "2026-08"},
		{name: "radio-activity-202613.csv", typ: ReportActivity},
		{name: "radio-activity-2026.csv", typ: ReportActivity},
		{name: "radio-escrow-202608.csv", month: "2026-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			month, typ := ParseMonthlyFilename(tt.name)
			if month != tt.month {
				t.Errorf("month = %q, want %q", month, tt.month)
			}
			if typ != tt.typ {
				t.Errorf("type = %q, want %q", typ, tt.typ)
			}
		})
	}
}

// TestParseMonthlyFilenameTakesTheLastMonth pins which month wins when a path
// or a copy suffix carries digits of its own.
func TestParseMonthlyFilenameTakesTheLastMonth(t *testing.T) {
	if month, _ := ParseMonthlyFilename("202501-backup/radio-activity-202608.csv"); month != "2026-08" {
		t.Errorf("month = %q, want 2026-08 (only the basename is considered)", month)
	}
	if month, _ := ParseMonthlyFilename("radio-activity-202608-202612.csv"); month != "2026-12" {
		t.Errorf("month = %q, want the last month in the name", month)
	}
	// A version suffix is not a month, so it must not displace the real one.
	if month, _ := ParseMonthlyFilename("radio-activity-202608-v2.csv"); month != "2026-08" {
		t.Errorf("month = %q, want 2026-08", month)
	}
}

// TestParseMonthlyReportDoesNotModifyInput guards the rule that the submitted
// body is the file's bytes: parsing must never touch them.
func TestParseMonthlyReportDoesNotModifyInput(t *testing.T) {
	raw := readFixture(t, "example-activity-202501-bom.csv")
	before := append([]byte(nil), raw...)
	if _, err := ParseMonthlyReport(raw); err != nil {
		t.Fatalf("ParseMonthlyReport: %v", err)
	}
	if !bytes.Equal(raw, before) {
		t.Error("ParseMonthlyReport modified its input")
	}
}
