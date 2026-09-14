package rri

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// XML namespaces used by the registry escrow report interfaces.
const (
	NamespaceRdeReport = "urn:ietf:params:xml:ns:rdeReport-1.0"
	NamespaceRdeHeader = "urn:ietf:params:xml:ns:rdeHeader-1.0"
	NamespaceIIRDEA    = "urn:ietf:params:xml:ns:iirdea-1.0"
)

// Deposit kinds defined by rdeReport-1.0.
const (
	KindFull = "FULL"
	KindIncr = "INCR"
	KindDiff = "DIFF"
)

// Errors returned by ParseRyEscrowReport when a required element is absent.
var (
	ErrReportIDMissing  = errors.New("rdeReport:id is missing from the report")
	ErrReportTLDMissing = errors.New("rdeHeader:tld is missing from the report")
)

// ObjectCount is a single <rdeHeader:count> element from a report header.
type ObjectCount struct {
	URI   string `json:"uri"`
	RCDN  string `json:"rcdn,omitempty"`
	Count int64  `json:"count"`
}

// ReportMeta holds the fields extracted from an RDE report for pre-flight
// validation. It is deliberately a subset of the document: a report is always
// submitted verbatim and is never re-serialized from this struct.
type ReportMeta struct {
	ID        string        `json:"id"`
	Version   int           `json:"version"`
	CrDate    time.Time     `json:"crDate"`
	Kind      string        `json:"kind"`
	Watermark time.Time     `json:"watermark"`
	TLD       string        `json:"tld"`
	Counts    []ObjectCount `json:"counts,omitempty"`
}

// xmlReport mirrors the subset of <rdeReport:report> that we inspect.
//
// Struct tags use the "<namespace-uri> <local-name>" form so that decoding
// works for documents that use a prefix (<rdeReport:id>) and for documents
// that put rdeReport-1.0 in the default namespace (<report xmlns="...">).
// Unprefixed attributes do not inherit the default namespace and are therefore
// tagged with a bare local name.
//
// These structs are decode-only. Go's encoding/xml cannot faithfully reproduce
// namespace prefixes on output, which is one more reason the submitted body is
// always the original file bytes rather than a re-marshaled document.
type xmlReport struct {
	XMLName   xml.Name  `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 report"`
	ID        string    `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 id"`
	Version   int       `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 version"`
	CrDate    string    `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 crDate"`
	Kind      string    `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 kind"`
	Watermark string    `xml:"urn:ietf:params:xml:ns:rdeReport-1.0 watermark"`
	Header    xmlHeader `xml:"urn:ietf:params:xml:ns:rdeHeader-1.0 header"`
}

type xmlHeader struct {
	// TLD is a slice so that a document with zero or several <tld> elements is
	// detectable rather than silently collapsing to the last one.
	TLD    []string   `xml:"urn:ietf:params:xml:ns:rdeHeader-1.0 tld"`
	Counts []xmlCount `xml:"urn:ietf:params:xml:ns:rdeHeader-1.0 count"`
}

type xmlCount struct {
	URI   string `xml:"uri,attr"`
	RCDN  string `xml:"rcdn,attr"`
	Value int64  `xml:",chardata"`
}

// utf8BOM is stripped before parsing; it is never stripped from a submitted body.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// newXMLDecoder returns a decoder that tolerates an explicit UTF-8 or US-ASCII
// encoding declaration and rejects anything else with an actionable message.
func newXMLDecoder(b []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(b, utf8BOM)))
	d.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "utf-8", "utf8", "us-ascii", "ascii":
			return input, nil
		default:
			return nil, fmt.Errorf("unsupported XML encoding %q; re-encode the report as UTF-8, or use --no-preflight with --id to submit it without local validation", charset)
		}
	}
	return d
}

// ParseRyEscrowReport extracts the fields needed for pre-flight validation from
// a serialized RDE report. It is a read-only inspection: the input bytes are
// not modified, and the report must be submitted exactly as read from disk.
func ParseRyEscrowReport(b []byte) (*ReportMeta, error) {
	var doc xmlReport
	if err := newXMLDecoder(b).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing RDE report: %w", err)
	}

	m := &ReportMeta{
		ID:      strings.TrimSpace(doc.ID),
		Version: doc.Version,
		Kind:    strings.ToUpper(strings.TrimSpace(doc.Kind)),
	}
	if m.ID == "" {
		return nil, ErrReportIDMissing
	}

	switch len(doc.Header.TLD) {
	case 0:
		return nil, ErrReportTLDMissing
	case 1:
		m.TLD = strings.TrimSpace(doc.Header.TLD[0])
	default:
		return nil, fmt.Errorf("report header contains %d <tld> elements, expected exactly one", len(doc.Header.TLD))
	}
	if m.TLD == "" {
		return nil, ErrReportTLDMissing
	}

	var err error
	if m.CrDate, err = parseReportTime("crDate", doc.CrDate); err != nil {
		return nil, err
	}
	if m.Watermark, err = parseReportTime("watermark", doc.Watermark); err != nil {
		return nil, err
	}

	for _, c := range doc.Header.Counts {
		m.Counts = append(m.Counts, ObjectCount{
			URI:   strings.TrimSpace(c.URI),
			RCDN:  strings.TrimSpace(c.RCDN),
			Count: c.Value,
		})
	}
	return m, nil
}

// parseReportTime parses an rdeReport timestamp, naming the field on failure.
func parseReportTime(field, v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, fmt.Errorf("rdeReport:%s is missing from the report", field)
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("rdeReport:%s: cannot parse %q as a timestamp", field, v)
}

// ValidationOptions controls the pre-flight checks performed on a report
// before it is submitted.
type ValidationOptions struct {
	// TLD is the configured TLD to check the report header against. Empty skips the check.
	TLD string
	// ID is the expected report identifier. Empty skips the check.
	ID string
	// Now pins the clock for the future-date checks. The zero value means time.Now().UTC().
	Now time.Time
}

// Validate performs local checks that pre-empt server-side rejections, so that
// an unusable report fails before a request is spent on it. All problems are
// reported together rather than one per run.
func (m *ReportMeta) Validate(opts ValidationOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	var errs []error

	if opts.TLD != "" && !strings.EqualFold(opts.TLD, m.TLD) {
		errs = append(errs, fmt.Errorf("report is for TLD %q but the client is configured for %q; ICANN would reject this with result code %d (if this is an IDN A-label/U-label difference, pass --no-preflight)", m.TLD, opts.TLD, ResultTLDMismatch))
	}
	if opts.ID != "" && opts.ID != m.ID {
		errs = append(errs, fmt.Errorf("report id is %q but %q was requested; the id in the URL and in the document must match (result code %d)", m.ID, opts.ID, ResultIDMismatch))
	}
	switch m.Kind {
	case KindFull, KindIncr, KindDiff:
	default:
		errs = append(errs, fmt.Errorf("rdeReport:kind is %q, expected one of %s, %s or %s", m.Kind, KindFull, KindIncr, KindDiff))
	}
	if m.CrDate.After(now) {
		errs = append(errs, fmt.Errorf("rdeReport:crDate %s is in the future (now %s); ICANN would reject this with result code %d, so check this machine's clock", m.CrDate.Format(time.RFC3339), now.Format(time.RFC3339), ResultDateInFuture))
	}
	if m.Watermark.After(now) {
		errs = append(errs, fmt.Errorf("rdeReport:watermark %s is in the future (now %s); ICANN would reject this with result code %d, so check this machine's clock", m.Watermark.Format(time.RFC3339), now.Format(time.RFC3339), ResultDateInFuture))
	}

	seen := make(map[ObjectCount]struct{}, len(m.Counts))
	var hasCSVDomain, hasRDEDomain bool
	for _, c := range m.Counts {
		key := ObjectCount{URI: c.URI, RCDN: c.RCDN}
		if _, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("duplicate <count> element for uri %q rcdn %q (result code %d)", c.URI, c.RCDN, ResultDuplicateCount))
		}
		seen[key] = struct{}{}

		switch {
		case strings.Contains(c.URI, "csvDomain"):
			hasCSVDomain = true
		case strings.Contains(c.URI, "rdeDomain"):
			hasRDEDomain = true
		}
	}
	if hasCSVDomain && hasRDEDomain {
		errs = append(errs, fmt.Errorf("report header counts both csvDomain and rdeDomain objects; only one domain model may be reported (result code %d)", ResultDuplicateDomainCount))
	}

	return errors.Join(errs...)
}
