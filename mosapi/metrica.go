package mosapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	base "github.com/onasunnymorning/icann-client/client"
)

// MetricaDomainListLatest represents the latest or dated METRICA domain list report.
type MetricaDomainListLatest struct {
	Version            int             `json:"version"`
	TLD                string          `json:"tld,omitempty"`
	IANAID             *int            `json:"ianaId,omitempty"`
	DomainListDate     string          `json:"domainListDate"`
	DomainsInZone      *int            `json:"domainsInZone,omitempty"`
	UniqueAbuseDomains int             `json:"uniqueAbuseDomains"`
	DomainListData     []MetricaThreat `json:"domainListData"`
	// Not from JSON; filled from the HTTP header when available.
	LastModified string `json:"-"`
}

// MetricaThreat represents abuse counts and domains for a threat type.
type MetricaThreat struct {
	ThreatType string   `json:"threatType"`
	Count      int      `json:"count"`
	Domains    []string `json:"domains"`
}

// MetricaDomainLists represents the list of METRICA reports available between optional dates.
type MetricaDomainLists struct {
	Version     int               `json:"version"`
	TLD         string            `json:"tld,omitempty"`
	IANAID      *int              `json:"ianaId,omitempty"`
	DomainLists []MetricaListInfo `json:"domainLists"`
}

// MetricaListInfo contains basic metadata for a METRICA report.
type MetricaListInfo struct {
	DomainListDate           string `json:"domainListDate"`
	DomainListGenerationDate string `json:"domainListGenerationDate"`
}

// GetMetricaLatest fetches the latest METRICA domain list report.
func (c *Client) GetMetricaLatest(ctx context.Context) (*MetricaDomainListLatest, error) {
	cfg := c.Config()
	path := fmt.Sprintf("/%s/%s/%s/metrica/domainList/latest", cfg.Entity, cfg.TLD, cfg.Version)
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	var out MetricaDomainListLatest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	out.LastModified = resp.Header.Get("Last-Modified")
	return &out, nil
}

// GetMetricaByDate fetches a METRICA report for a specific date (YYYY-MM-DD).
func (c *Client) GetMetricaByDate(ctx context.Context, date string) (*MetricaDomainListLatest, error) {
	cfg := c.Config()
	path := fmt.Sprintf("/%s/%s/%s/metrica/domainList/%s", cfg.Entity, cfg.TLD, cfg.Version, url.PathEscape(date))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	var out MetricaDomainListLatest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	out.LastModified = resp.Header.Get("Last-Modified")
	return &out, nil
}

// ListMetricaReports lists available METRICA reports, optionally filtered by startDate and endDate (YYYY-MM-DD).
func (c *Client) ListMetricaReports(ctx context.Context, startDate, endDate string) (*MetricaDomainLists, error) {
	cfg := c.Config()
	basePath := fmt.Sprintf("/%s/%s/%s/metrica/domainLists", cfg.Entity, cfg.TLD, cfg.Version)
	// Build query parameters if provided
	u, _ := url.Parse(basePath)
	q := u.Query()
	if startDate != "" {
		q.Set("startDate", startDate)
	}
	if endDate != "" {
		q.Set("endDate", endDate)
	}
	u.RawQuery = q.Encode()

	req, err := c.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	var out MetricaDomainLists
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
