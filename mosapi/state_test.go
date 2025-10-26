package mosapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	base "github.com/onasunnymorning/icann-client/client"
)

func TestGetStateResponse(t *testing.T) {
	// Mock MOSAPI endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/" + base.EntityRegistry + "/example/" + base.V2 + "/monitoring/state"
		if r.URL.Path != expectedPath {
			t.Fatalf("unexpected path: %s (want %s)", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := StateResponse{
			TLD:             "example",
			LastUpdateApiDb: 1234567890,
			Status:          "Up",
			TestedServices: map[string]TestedService{
				"DNS": {
					Status:             "Up",
					EmergencyThreshold: 0,
					Incidents:          []Incident{},
				},
			},
			Version: 2,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := base.Config{
		TLD:         "example",
		AuthType:    base.AUTH_TYPE_BASIC,
		Username:    "u",
		Password:    "p",
		Environment: base.ENV_PROD,
		Version:     base.V2,
		Entity:      base.EntityRegistry,
	}

	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New(mosapi) error: %v", err)
	}
	// Point the base client to our test server
	if err := cli.WithBaseURL(srv.URL); err != nil {
		t.Fatalf("WithBaseURL: %v", err)
	}

	got, err := cli.GetStateResponse(context.Background())
	if err != nil {
		t.Fatalf("GetStateResponse: %v", err)
	}
	if got.TLD != "example" {
		t.Fatalf("unexpected TLD: %s", got.TLD)
	}
	if !got.AllServicesUp() {
		t.Fatalf("expected all services up")
	}
	if got.HasIncidents() {
		t.Fatalf("expected no incidents")
	}
}
