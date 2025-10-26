package mosapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

func newTestMOSAPI(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	cfg := base.Config{TLD: "example", Environment: base.ENV_PROD, Version: base.V2, Entity: base.EntityRegistry, AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := c.WithBaseURL(srv.URL); err != nil {
		t.Fatalf("with base url: %v", err)
	}
	return c
}

func TestGetMetricaLatest_PathAndDecode(t *testing.T) {
	wantPath := "/ry/example/v2/metrica/domainList/latest"
	lastMod := "Wed, 01 Jan 2025 00:00:00 GMT"
	payload := MetricaDomainListLatest{
		Version:            2,
		TLD:                "example",
		DomainListDate:     "2025-01-01",
		UniqueAbuseDomains: 14,
		DomainListData:     []MetricaThreat{{ThreatType: "spam", Count: 2, Domains: []string{"a.example", "b.example"}}},
	}
	c := newTestMOSAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("path = %s, want %s", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Last-Modified", lastMod)
		json.NewEncoder(w).Encode(payload)
	})

	got, err := c.GetMetricaLatest(context.Background())
	if err != nil {
		t.Fatalf("GetMetricaLatest: %v", err)
	}
	if got.LastModified != lastMod {
		t.Fatalf("LastModified = %q, want %q", got.LastModified, lastMod)
	}
	if got.Version != 2 || got.TLD != "example" || got.DomainListDate != "2025-01-01" {
		t.Fatalf("unexpected fields: %+v", got)
	}
	if len(got.DomainListData) != 1 || got.DomainListData[0].ThreatType != "spam" || got.DomainListData[0].Count != 2 {
		t.Fatalf("unexpected domainListData: %+v", got.DomainListData)
	}
}

func TestGetMetricaByDate_PathAndDecode(t *testing.T) {
	wantPath := "/ry/example/v2/metrica/domainList/2024-02-20"
	payload := MetricaDomainListLatest{
		Version:            2,
		TLD:                "example",
		DomainListDate:     "2024-02-20",
		UniqueAbuseDomains: 1,
		DomainListData:     []MetricaThreat{{ThreatType: "phishing", Count: 1, Domains: []string{"x.example"}}},
	}
	c := newTestMOSAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("path = %s, want %s", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(payload)
	})

	got, err := c.GetMetricaByDate(context.Background(), "2024-02-20")
	if err != nil {
		t.Fatalf("GetMetricaByDate: %v", err)
	}
	if got.DomainListDate != "2024-02-20" || got.Version != 2 {
		t.Fatalf("unexpected fields: %+v", got)
	}
	if len(got.DomainListData) != 1 || got.DomainListData[0].ThreatType != "phishing" {
		t.Fatalf("unexpected domainListData: %+v", got.DomainListData)
	}
}

func TestListMetricaReports_PathAndDecode_NoParams(t *testing.T) {
	wantPath := "/ry/example/v2/metrica/domainLists"
	payload := MetricaDomainLists{
		Version: 2,
		TLD:     "example",
		DomainLists: []MetricaListInfo{
			{DomainListDate: "2018-12-12", DomainListGenerationDate: "2018-12-13T23:20:50.52Z"},
			{DomainListDate: "2018-12-13", DomainListGenerationDate: "2018-12-14T23:20:51.52Z"},
		},
	}
	c := newTestMOSAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath || r.URL.RawQuery != "" {
			t.Fatalf("path/query = %s?%s, want %s", r.URL.Path, r.URL.RawQuery, wantPath)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(payload)
	})

	got, err := c.ListMetricaReports(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListMetricaReports: %v", err)
	}
	if got.Version != 2 || got.TLD != "example" || len(got.DomainLists) != 2 {
		t.Fatalf("unexpected fields: %+v", got)
	}
}

func TestListMetricaReports_WithParams(t *testing.T) {
	wantPath := "/ry/example/v2/metrica/domainLists"
	wantQ := "endDate=2025-01-31&startDate=2025-01-01"
	payload := MetricaDomainLists{Version: 2, TLD: "example", DomainLists: []MetricaListInfo{}}
	c := newTestMOSAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("path = %s, want %s", r.URL.Path, wantPath)
		}
		if r.URL.RawQuery != wantQ && r.URL.RawQuery != "startDate=2025-01-01&endDate=2025-01-31" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(payload)
	})

	got, err := c.ListMetricaReports(context.Background(), "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("ListMetricaReports: %v", err)
	}
	if got.Version != 2 || got.TLD != "example" {
		t.Fatalf("unexpected fields: %+v", got)
	}
}
