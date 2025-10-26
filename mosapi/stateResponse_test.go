package mosapi

import (
	"testing"
)

func TestAllServicesUp(t *testing.T) {
	tests := []struct {
		name     string
		response StateResponse
		expected bool
	}{
		{
			name: "All services up",
			response: StateResponse{
				TestedServices: map[string]TestedService{
					"service1": {Status: "Up"},
					"service2": {Status: "Up"},
				},
			},
			expected: true,
		},
		{
			name: "One service down",
			response: StateResponse{
				TestedServices: map[string]TestedService{
					"service1": {Status: "Up"},
					"service2": {Status: "Down"},
				},
			},
			expected: false,
		},
		{
			name: "No services",
			response: StateResponse{
				TestedServices: map[string]TestedService{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.AllServicesUp()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsUp(t *testing.T) {
	tests := []struct {
		name     string
		service  TestedService
		expected bool
	}{
		{
			name: "Service is up",
			service: TestedService{
				Status: "Up",
			},
			expected: true,
		},
		{
			name: "Service is disabled",
			service: TestedService{
				Status: "Disabled",
			},
			expected: true,
		},
		{
			name: "Service is down",
			service: TestedService{
				Status: "Down",
			},
			expected: false,
		},
		{
			name: "Service is UP-inconclusive-no-data",
			service: TestedService{
				Status: "UP-inconclusive-no-data",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.service.IsUp()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
func TestHasAlerts(t *testing.T) {
	tests := []struct {
		name     string
		service  TestedService
		expected bool
	}{
		{
			name: "Service has alerts",
			service: TestedService{
				Incidents: []Incident{
					{IncidentID: "1"},
				},
			},
			expected: true,
		},
		{
			name: "Service has no alerts",
			service: TestedService{
				Incidents: []Incident{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.service.HasIncidents()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
func TestStateResponseHasAlerts(t *testing.T) {
	tests := []struct {
		name     string
		response StateResponse
		expected bool
	}{
		{
			name: "No services with alerts",
			response: StateResponse{
				TestedServices: map[string]TestedService{
					"service1": {Incidents: []Incident{}},
					"service2": {Incidents: []Incident{}},
				},
			},
			expected: false,
		},
		{
			name: "One service with alerts",
			response: StateResponse{
				TestedServices: map[string]TestedService{
					"service1": {Incidents: []Incident{}},
					"service2": {Incidents: []Incident{{IncidentID: "1"}}},
				},
			},
			expected: true,
		},
		{
			name: "Multiple services with alerts",
			response: StateResponse{
				TestedServices: map[string]TestedService{
					"service1": {Incidents: []Incident{{IncidentID: "1"}}},
					"service2": {Incidents: []Incident{{IncidentID: "2"}}},
				},
			},
			expected: true,
		},
		{
			name: "No services",
			response: StateResponse{
				TestedServices: map[string]TestedService{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.HasIncidents()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
