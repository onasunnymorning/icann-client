package client

import "fmt"

// HTTPError represents a non-2xx HTTP response.
// It includes the status code, method, URL, and the response body for programmatic inspection.
type HTTPError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("http error: %d %s %s - %s", e.StatusCode, e.Method, e.URL, e.Body)
	}
	return fmt.Sprintf("http error: %d %s %s", e.StatusCode, e.Method, e.URL)
}
