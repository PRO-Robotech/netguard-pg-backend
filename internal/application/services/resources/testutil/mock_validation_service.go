package testutil

import (
	"netguard-pg-backend/internal/domain/ports"
)

// MockValidationService is a simple mock that implements validation behavior
type MockValidationService struct {
	registry ports.Registry
}

// NewMockValidationService creates a mock ValidationService for testing
func NewMockValidationService(registry ports.Registry) *MockValidationService {
	return &MockValidationService{
		registry: registry,
	}
}
