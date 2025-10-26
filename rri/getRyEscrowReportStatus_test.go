package rri

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestCheckRyEscrowReport(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name           string
		statusCode     int
		expectedStatus string
		expectedError  string
		mockError      error
	}{
		{
			name:           "Report received (status 200)",
			statusCode:     http.StatusOK,
			expectedStatus: RY_RDEReport_RECEIVED,
			expectedError:  "",
			mockError:      nil,
		},
		{
			name:           "Report pending (status 404)",
			statusCode:     http.StatusNotFound,
			expectedStatus: RY_RDEReport_PENDING,
			expectedError:  "",
			mockError:      nil,
		},
		{
			name:           "Unexpected status code (status 500)",
			statusCode:     http.StatusInternalServerError,
			expectedStatus: "",
			expectedError:  "unexpected status code: 500",
			mockError:      nil,
		},
		{
			name:           "HTTP client error",
			statusCode:     0,
			expectedStatus: "",
			expectedError:  "mock error",
			mockError:      errors.New("mock error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// Create a mock HTTP client
			mockTransport := &mockRoundTripper{
				response: &http.Response{
					StatusCode: tc.statusCode,
					Body:       http.NoBody,
					// Include a Request to prevent nil pointer dereference
					Request: &http.Request{},
				},
				err: tc.mockError,
			}
			// Create RRI client using shared base client and override transport
			cfg := base.Config{TLD: "com", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"}
			rriClient, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			// Override base URL and transport for isolation
			_ = rriClient.WithBaseURL("https://example.com")
			rriClient.HTTPClient = &http.Client{Transport: mockTransport}

			date := time.Now()

			// Act
			statusReport, err := rriClient.GetRyEscrowReportStatus(context.Background(), date)

			// Assert
			if tc.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error '%v', got nil", tc.expectedError)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%v', got '%v'", tc.expectedError, err)
				}
				if statusReport != nil {
					t.Errorf("Expected nil statusReport, got %v", statusReport)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got '%v'", err)
				}
				if statusReport == nil {
					t.Errorf("Expected statusReport, got nil")
				} else if statusReport.Status != tc.expectedStatus {
					t.Errorf("Expected status '%s', got '%s'", tc.expectedStatus, statusReport.Status)
				}
			}
		})
	}
}
