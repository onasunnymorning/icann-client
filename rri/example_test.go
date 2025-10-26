package rri_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
	"github.com/onasunnymorning/icann-client/rri"
)

func ExampleClient_GetRyEscrowReportStatus() {
	// Fake RRI escrow status endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rri/escrow/ry/example/2025-10-22/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := base.Config{TLD: "example", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
	rc, _ := rri.New(cfg)
	_ = rc.WithBaseURL(srv.URL)
	st, _ := rc.GetRyEscrowReportStatus(context.Background(), time.Date(2025, 10, 22, 0, 0, 0, 0, time.UTC))
	fmt.Println(st.Status)
	// Output: received
}
