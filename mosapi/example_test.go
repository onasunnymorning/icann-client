package mosapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/mosapi"
)

func ExampleClient_GetStateResponse() {
	// Fake MOSAPI server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ry/example/v2/monitoring/state" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{
			"tld": "example", "lastUpdateApiDatabase": 0, "status": "Up", "testedServices": map[string]any{}, "version": 2,
		})
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p", Environment: base.ENV_PROD, Version: base.V2, Entity: base.EntityRegistry}
	c, _ := mosapi.New(cfg)
	_ = c.WithBaseURL(srv.URL)

	sr, _ := c.GetStateResponse(context.Background())
	fmt.Println(sr.Status)
	// Output: Up
}

func ExampleClient_GetMetricaLatest() {
	// Fake METRICA latest endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ry/example/v2/metrica/domainList/latest" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Last-Modified", "Wed, 01 Jan 2025 00:00:00 GMT")
		json.NewEncoder(w).Encode(map[string]any{
			"version": 2, "tld": "example", "domainListDate": "2025-01-01", "uniqueAbuseDomains": 0, "domainListData": []any{},
		})
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	c, _ := mosapi.New(cfg)
	_ = c.WithBaseURL(srv.URL)
	out, _ := c.GetMetricaLatest(context.Background())
	fmt.Println(out.Version)
	// Output: 2
}
