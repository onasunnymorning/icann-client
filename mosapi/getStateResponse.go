package mosapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	base "github.com/onasunnymorning/icann-client/client"
)

// StateResponse represents the generic shape of the monitoring state endpoint.
// If a formal schema is available, replace this with a concrete struct.
// GetStateResponse requests the MOSAPI monitoring state and returns the parsed response.
// It uses the shared base client wiring (auth, base URL, timeouts).
func (c *Client) GetStateResponse(ctx context.Context) (*StateResponse, error) {
	cfg := c.Config()
	// Build path per MOSAPI spec: /<entity>/<tld or registrar ID>/<version>/monitoring/state
	path := fmt.Sprintf("/%s/%s/%s/monitoring/state", cfg.Entity, cfg.TLD, cfg.Version)
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Include the URL to aid debugging; return a typed HTTPError for programmatic handling.
		return nil, &base.HTTPError{StatusCode: resp.StatusCode, Method: req.Method, URL: req.URL.String()}
	}
	var out StateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
