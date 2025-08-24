package validation

import (
	"reflect"
	"testing"

	"netguard-pg-backend/internal/domain/models"
)

func TestParsePortRanges(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		expected    []models.PortRange
		expectError bool
	}{
		{
			name:        "Single port",
			port:        "80",
			expected:    []models.PortRange{{Start: 80, End: 80}},
			expectError: false,
		},
		{
			name:        "Port range",
			port:        "8080-8090",
			expected:    []models.PortRange{{Start: 8080, End: 8090}},
			expectError: false,
		},
		{
			name:        "Comma-separated list of ports",
			port:        "80, 443",
			expected:    []models.PortRange{{Start: 80, End: 80}, {Start: 443, End: 443}},
			expectError: false,
		},
		{
			name:        "Comma-separated list with range",
			port:        "80, 8080-8090",
			expected:    []models.PortRange{{Start: 80, End: 80}, {Start: 8080, End: 8090}},
			expectError: false,
		},
		{
			name:        "Empty port",
			port:        "",
			expectError: true,
		},
		{
			name:        "Invalid port format",
			port:        "invalid",
			expectError: true,
		},
		{
			name:        "Invalid port in list",
			port:        "80, invalid",
			expectError: true,
		},
		{
			name:        "Invalid port range format",
			port:        "80-invalid",
			expectError: true,
		},
		{
			name:        "Start port greater than end port",
			port:        "9000-8000",
			expectError: true,
		},
		{
			name:        "Port out of range",
			port:        "70000",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePortRanges(tt.port)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for port %s, but got nil", tt.port)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for port %s: %v", tt.port, err)
				}
				if !reflect.DeepEqual(result, tt.expected) {
					t.Errorf("Expected port ranges %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestParsePortRange(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		expected    models.PortRange
		expectError bool
	}{
		{
			name:        "Single port",
			port:        "80",
			expected:    models.PortRange{Start: 80, End: 80},
			expectError: false,
		},
		{
			name:        "Port range",
			port:        "8080-8090",
			expected:    models.PortRange{Start: 8080, End: 8090},
			expectError: false,
		},
		{
			name:        "Empty port",
			port:        "",
			expectError: true,
		},
		{
			name:        "Invalid port format",
			port:        "invalid",
			expectError: true,
		},
		{
			name:        "Invalid port range format",
			port:        "80-invalid",
			expectError: true,
		},
		{
			name:        "Start port greater than end port",
			port:        "9000-8000",
			expectError: true,
		},
		{
			name:        "Port out of range",
			port:        "70000",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePortRange(tt.port)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for port %s, but got nil", tt.port)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for port %s: %v", tt.port, err)
				}
				if result.Start != tt.expected.Start || result.End != tt.expected.End {
					t.Errorf("Expected port range %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestDoPortRangesOverlap(t *testing.T) {
	tests := []struct {
		name     string
		rangeA   models.PortRange
		rangeB   models.PortRange
		expected bool
	}{
		{
			name:     "Non-overlapping ranges",
			rangeA:   models.PortRange{Start: 80, End: 90},
			rangeB:   models.PortRange{Start: 100, End: 110},
			expected: false,
		},
		{
			name:     "Overlapping ranges",
			rangeA:   models.PortRange{Start: 80, End: 100},
			rangeB:   models.PortRange{Start: 90, End: 110},
			expected: true,
		},
		{
			name:     "Range A contains Range B",
			rangeA:   models.PortRange{Start: 80, End: 120},
			rangeB:   models.PortRange{Start: 90, End: 110},
			expected: true,
		},
		{
			name:     "Range B contains Range A",
			rangeA:   models.PortRange{Start: 90, End: 110},
			rangeB:   models.PortRange{Start: 80, End: 120},
			expected: true,
		},
		{
			name:     "Adjacent ranges",
			rangeA:   models.PortRange{Start: 80, End: 90},
			rangeB:   models.PortRange{Start: 91, End: 100},
			expected: false,
		},
		{
			name:     "Touching ranges",
			rangeA:   models.PortRange{Start: 80, End: 90},
			rangeB:   models.PortRange{Start: 90, End: 100},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DoPortRangesOverlap(tt.rangeA, tt.rangeB)
			if result != tt.expected {
				t.Errorf("Expected DoPortRangesOverlap(%v, %v) = %v, got %v",
					tt.rangeA, tt.rangeB, tt.expected, result)
			}
		})
	}
}
