package client

import "fmt"

// HTTPError represents a non-2xx HTTP response.
// It includes the status code, method, and URL for programmatic inspection.
type HTTPError struct {
	StatusCode int
	Method     string
	URL        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error: %d %s %s", e.StatusCode, e.Method, e.URL)
}
