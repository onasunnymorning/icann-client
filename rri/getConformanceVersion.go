package rri

import (
	"context"
	"encoding/xml"
	"net/http"
)

// NamespaceRRIConformance is the XML namespace of the conformance document.
const NamespaceRRIConformance = "urn:ietf:params:xml:ns:rriConformance-1.0"

// Specifications ICANN is conformant to when it answers the conformance
// endpoint with HTTP 404. The draft defines the 404 this way, so it is a
// documented answer rather than a missing resource.
var conformanceBefore404 = []string{
	"draft-lozano-icann-registry-interfaces-25",
	"draft-icann-registrar-interfaces-15",
}

// Conformance reports which interface specifications ICANN implements.
type Conformance struct {
	Specifications []string `json:"specifications"`
	// Inferred is true when ICANN answered 404, which the draft defines as
	// conformance to the two specifications that predate the endpoint.
	Inferred bool `json:"inferred,omitempty"`
}

// xmlConformance is a decode-only mirror of the response document. The tags
// are namespace-qualified so that a document using a different prefix, or
// declaring the namespace as the default, decodes identically.
type xmlConformance struct {
	XMLName        xml.Name `xml:"urn:ietf:params:xml:ns:rriConformance-1.0 rriConformance"`
	Specifications []string `xml:"urn:ietf:params:xml:ns:rriConformance-1.0 specification"`
}

// GetConformanceVersion reports which RRI specifications ICANN implements, per
// draft-lozano-icann-registry-interfaces Section 7.
//
// An HTTP 404 is not a failure here. The draft defines it as meaning the
// server predates this endpoint and conforms to
// draft-lozano-icann-registry-interfaces-25 and
// draft-icann-registrar-interfaces-15, so those are returned with Inferred set
// rather than an error.
func (c *Client) GetConformanceVersion(ctx context.Context) (*Conformance, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, "/info/status/conformance-version", nil)
	if err != nil {
		return nil, err
	}
	// No Accept header. The draft requires none, and ICANN answered
	// "406 Not Acceptable" to Accept: text/xml on the reporting status
	// endpoint, so constraining the response is strictly worse than letting
	// the server send what it sends. GetRyEscrowReportStatus, the one RRI GET
	// proven against production, sends none either. doXMLGet parses the body
	// as XML regardless of the Content-Type it arrives with.

	raw, status, err := c.doGet(req)
	if status == http.StatusNotFound {
		return &Conformance{Specifications: append([]string(nil), conformanceBefore404...), Inferred: true}, nil
	}
	if err != nil {
		return nil, err
	}

	// The draft specifies XML here. Production answers 404, so this branch is
	// unexercised against ICANN; note that the reporting summary, the one read
	// endpoint that does answer, serves JSON instead.
	var doc xmlConformance
	if err := newXMLDecoder(raw).Decode(&doc); err != nil {
		return nil, decodeError(req, "a conformance document", raw, err)
	}
	return &Conformance{Specifications: doc.Specifications}, nil
}
