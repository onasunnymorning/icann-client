package rri

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ReportType identifies which Specification 3 monthly report a file holds.
type ReportType string

// The two monthly reports defined by Specification 3 of the gTLD Base Registry
// Agreement, each with its own submission interface.
const (
	// ReportTransactions is the Per-Registrar Transactions Report (Specification 3, Section 1).
	ReportTransactions ReportType = "transactions"
	// ReportActivity is the Registry Functions Activity Report (Specification 3, Section 2).
	ReportActivity ReportType = "activity"
)

// totalsLabel is the value of the first field of the totals line that closes a
// Per-Registrar Transactions Report.
const totalsLabel = "Totals"

// Errors returned while resolving a monthly report's type and month.
var (
	ErrMonthlyTypeUnknown  = errors.New("cannot tell whether this is a transactions or an activity report")
	ErrMonthlyMonthMissing = errors.New("cannot determine the report month")
)

// MonthlyMeta holds the fields extracted from a Specification 3 monthly report
// for pre-flight validation. It is a read-only view: the report is always
// submitted exactly as read from disk and is never re-serialized from here.
type MonthlyMeta struct {
	Type      ReportType `json:"type,omitempty"`
	TLD       string     `json:"tld,omitempty"`
	Columns   []string   `json:"columns"`
	Rows      int        `json:"rows"`
	HasTotals bool       `json:"hasTotals"`
	HasBOM    bool       `json:"hasBOM,omitempty"`

	// data holds the records between the header and the totals line, and
	// totals the totals line itself when present. Both are kept so that
	// Validate can re-add the columns without re-reading the file.
	data   [][]string
	totals []string
}

// ParseMonthlyReport reads a Specification 3 monthly report and extracts what
// is needed for pre-flight validation. The input bytes are not modified.
func ParseMonthlyReport(b []byte) (*MonthlyMeta, error) {
	if !utf8.Valid(b) {
		return nil, fmt.Errorf("report is not valid UTF-8; ICANN would reject it with result code %d (US-ASCII is also accepted)", ResultNotUTF8)
	}

	m := &MonthlyMeta{HasBOM: bytes.HasPrefix(b, utf8BOM)}

	// The BOM is stripped for parsing only. It is never stripped from the body
	// that gets submitted.
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, utf8BOM)))
	r.FieldsPerRecord = 0 // the header pins the field count; a ragged row errors
	r.LazyQuotes = false
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing CSV: %w (ICANN would reject this with result code %d)", err, ResultBadRequest)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("report has %d CSV records; expected a header line and at least one data line (result code %d)", len(records), ResultBadRequest)
	}

	m.Columns = make([]string, 0, len(records[0]))
	for _, c := range records[0] {
		m.Columns = append(m.Columns, strings.TrimSpace(c))
	}

	rows := records[1:]
	if last := rows[len(rows)-1]; len(last) > 0 && strings.EqualFold(strings.TrimSpace(last[0]), totalsLabel) {
		m.HasTotals = true
		m.totals = last
		rows = rows[:len(rows)-1]
	}
	m.data = rows
	m.Rows = len(rows)

	if t, ok := DetectReportType(m.Columns); ok {
		m.Type = t
	}
	if i := columnIndex(m.Columns, "tld"); i >= 0 && len(m.data) > 0 && i < len(m.data[0]) {
		m.TLD = strings.TrimSpace(m.data[0][i])
	}
	return m, nil
}

// DetectReportType identifies a monthly report from its CSV header line. It
// reports false when the columns match neither report, or match both.
func DetectReportType(columns []string) (ReportType, bool) {
	has := func(names ...string) bool {
		for _, n := range names {
			if columnIndex(columns, n) >= 0 {
				return true
			}
		}
		return false
	}
	// Distinctive columns rather than an exhaustive list: Specification 3 has
	// been amended over time and column sets differ between agreements, so
	// matching on the full set would reject valid reports.
	transactions := has("registrar-name", "iana-id")
	activity := has("whois-43-queries", "dns-udp-queries-received", "srs-dom-create")

	switch {
	case transactions && !activity:
		return ReportTransactions, true
	case activity && !transactions:
		return ReportActivity, true
	default:
		return "", false
	}
}

// ParseMonthlyFilename extracts the month and report type from a report
// filename. An empty return value means that part could not be determined.
//
// It accepts the Specification 3 convention ("example-transactions-202501.csv",
// "example-activity-202501.csv") and the looser shapes that providers actually
// hand over, such as "registrar-transactions-2026-08.csv" or
// "registry-functions-activity-2026-08.csv": the type is any "transactions" or
// "activity" token anywhere in the name, and the month is the last YYYYMM or
// YYYY-MM in it. The returned month is normalized to YYYY-MM.
//
// The TLD is deliberately not returned. It comes from the client configuration,
// and the tld column inside the report is what gets validated against it;
// reading it from a filename like "registrar-transactions-2026-08.csv" would
// yield "registrar", which is worse than nothing.
func ParseMonthlyFilename(name string) (month string, typ ReportType) {
	stem := filepath.Base(name)
	if ext := filepath.Ext(stem); strings.EqualFold(ext, ".csv") {
		stem = strings.TrimSuffix(stem, ext)
	}
	tokens := strings.FieldsFunc(strings.ToLower(stem), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})

	hasToken := func(want string) bool {
		for _, t := range tokens {
			if t == want {
				return true
			}
		}
		return false
	}
	transactions, activity := hasToken(string(ReportTransactions)), hasToken(string(ReportActivity))
	switch {
	case transactions && !activity:
		typ = ReportTransactions
	case activity && !transactions:
		typ = ReportActivity
	}

	// Take the last month in the name: a directory or copy suffix is far more
	// likely to precede the month than to follow it.
	for i, t := range tokens {
		if m, ok := normalizeMonth(t); ok {
			month = m
			continue
		}
		if i+1 < len(tokens) {
			if m, ok := normalizeMonth(t + tokens[i+1]); ok {
				month = m
			}
		}
	}
	return month, typ
}

// normalizeMonth accepts a bare YYYYMM and returns it as YYYY-MM.
func normalizeMonth(v string) (string, bool) {
	if len(v) != 6 {
		return "", false
	}
	if _, err := time.Parse("200601", v); err != nil {
		return "", false
	}
	return v[:4] + "-" + v[4:], true
}

// MonthlyValidationOptions controls the pre-flight checks performed on a
// monthly report before it is submitted.
type MonthlyValidationOptions struct {
	// TLD is the configured TLD to check the report's tld column against. Empty skips the check.
	TLD string
	// Month is the YYYY-MM the report will be submitted for. Empty skips the future-month check.
	Month string
	// Type overrides the detected report type. Empty uses the detected one.
	Type ReportType
	// Now pins the clock for the future-month check. The zero value means time.Now().UTC().
	Now time.Time
}

// Validate performs local checks that pre-empt server-side rejections, so that
// an unusable report fails before an authenticated request is spent on it. All
// problems are reported together rather than one per run.
func (m *MonthlyMeta) Validate(opts MonthlyValidationOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	typ := opts.Type
	if typ == "" {
		typ = m.Type
	}

	var errs []error

	if len(m.Columns) == 0 {
		errs = append(errs, fmt.Errorf("the CSV header line is empty (result code %d)", ResultBadRequest))
	}
	seen := make(map[string]struct{}, len(m.Columns))
	for _, c := range m.Columns {
		key := strings.ToLower(c)
		if _, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("duplicate column %q in the CSV header line (result code %d)", c, ResultBadRequest))
		}
		seen[key] = struct{}{}
	}
	if m.Rows == 0 {
		errs = append(errs, fmt.Errorf("report has no data lines (result code %d)", ResultBadRequest))
	}

	if opts.TLD != "" && m.TLD != "" && !strings.EqualFold(opts.TLD, m.TLD) {
		errs = append(errs, fmt.Errorf("report is for TLD %q but the client is configured for %q (result code %d; for an IDN pass --no-preflight)", m.TLD, opts.TLD, ResultBadRequest))
	}

	if opts.Month != "" {
		if t, err := time.Parse("2006-01", opts.Month); err != nil {
			errs = append(errs, fmt.Errorf("month %q is not in YYYY-MM form; ICANN would reject the URL with result code %d", opts.Month, ResultInvalidDateInURL))
		} else if t.After(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)) {
			errs = append(errs, fmt.Errorf("month %s is in the future (now %s); ICANN would reject this with result code %d, so check this machine's clock", opts.Month, now.Format("2006-01"), ResultDateInFuture))
		}
	}

	numeric := m.numericColumns()
	errs = append(errs, m.checkNegatives(numeric)...)

	if typ == ReportTransactions {
		errs = append(errs, m.checkTotals(numeric)...)
	}

	return errors.Join(errs...)
}

// numericColumns reports, per column index, whether every non-empty cell in
// the data lines parses as an integer. Only such columns are summed and
// range-checked, so that free-text columns like registrar-name are left alone.
func (m *MonthlyMeta) numericColumns() []bool {
	numeric := make([]bool, len(m.Columns))
	for i := range numeric {
		if i == 0 {
			continue // the key column (tld or registrar-name) is never a measure
		}
		any := false
		ok := true
		for _, row := range m.data {
			if i >= len(row) {
				continue
			}
			v := strings.TrimSpace(row[i])
			if v == "" {
				continue
			}
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				ok = false
				break
			}
			any = true
		}
		numeric[i] = any && ok
	}
	return numeric
}

// checkNegatives pre-empts result code 2003.
func (m *MonthlyMeta) checkNegatives(numeric []bool) []error {
	var errs []error
	for i, isNum := range numeric {
		if !isNum {
			continue
		}
		for r, row := range m.data {
			if i >= len(row) {
				continue
			}
			v := strings.TrimSpace(row[i])
			if v == "" {
				continue
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil && n < 0 {
				errs = append(errs, fmt.Errorf("line %d: column %q is %d; negative values are not permitted (result code %d)", r+2, m.column(i), n, ResultNegativeValues))
			}
		}
	}
	return errs
}

// checkTotals re-adds each numeric column and compares it with the totals line,
// pre-empting result codes 2001, 2103 and 2101.
func (m *MonthlyMeta) checkTotals(numeric []bool) []error {
	if !m.HasTotals {
		return []error{fmt.Errorf("a Per-Registrar Transactions Report must end with a totals line whose first field reads %q (result code %d)", totalsLabel, ResultBadRequest)}
	}

	var errs []error
	if len(m.totals) > 1 && strings.TrimSpace(m.totals[1]) != "" {
		errs = append(errs, fmt.Errorf("the second field of the totals line is %q; it must be empty (result code %d)", strings.TrimSpace(m.totals[1]), ResultTotalsLineNotEmpty))
	}

	for i, isNum := range numeric {
		if !isNum || i >= len(m.totals) {
			continue
		}
		stated := strings.TrimSpace(m.totals[i])
		if stated == "" {
			continue
		}
		want, err := strconv.ParseInt(stated, 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("totals line: column %q is %q, which is not a number (result code %d)", m.column(i), stated, ResultBadRequest))
			continue
		}
		var got int64
		for _, row := range m.data {
			if i >= len(row) {
				continue
			}
			if v := strings.TrimSpace(row[i]); v != "" {
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					continue
				}
				got += n
			}
		}
		if got != want {
			errs = append(errs, fmt.Errorf("column %q: totals line says %d, but the %d data lines sum to %d (result code %d)", m.column(i), want, len(m.data), got, ResultIncorrectTotals))
		}
	}
	return errs
}

// column names column i, falling back to its position when the header is short.
func (m *MonthlyMeta) column(i int) string {
	if i < len(m.Columns) && m.Columns[i] != "" {
		return m.Columns[i]
	}
	return "column " + strconv.Itoa(i+1)
}

// columnIndex returns the position of the named column, or -1.
func columnIndex(columns []string, name string) int {
	for i, c := range columns {
		if strings.EqualFold(strings.TrimSpace(c), name) {
			return i
		}
	}
	return -1
}
