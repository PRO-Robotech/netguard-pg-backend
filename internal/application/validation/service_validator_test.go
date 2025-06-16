package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestServiceValidator_ValidateExists tests the ValidateExists method of ServiceValidator
func TestServiceValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a service for the test ID
	mockReader := &MockReaderForServiceValidator{
		serviceExists: true,
		serviceID:     "test-service",
	}

	validator := validation.NewServiceValidator(mockReader)
	serviceID := models.NewResourceIdentifier("test-service")

	// Test when service exists
	err := validator.ValidateExists(context.Background(), serviceID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service does not exist
	mockReader.serviceExists = false
	err = validator.ValidateExists(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestServiceValidator_CheckDependencies tests the CheckDependencies method of ServiceValidator
func TestServiceValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForServiceValidator{
		serviceID:      "test-service",
		hasAliases:     false,
		hasBindings:    false,
	}

	validator := validation.NewServiceValidator(mockReader)
	serviceID := models.NewResourceIdentifier("test-service")

	// Test when no dependencies exist
	err := validator.CheckDependencies(context.Background(), serviceID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service alias dependency exists
	mockReader.hasAliases = true
	err = validator.CheckDependencies(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error for service alias dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Test when address group binding dependency exists
	mockReader.hasAliases = false
	mockReader.hasBindings = true
	err = validator.CheckDependencies(context.Background(), serviceID)
	if err == nil {
		t.Error("Expected error for address group binding dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}
}

// MockReaderForServiceValidator is a specialized mock for testing ServiceValidator
type MockReaderForServiceValidator struct {
	serviceExists bool
	serviceID     string
	hasAliases    bool
	hasBindings   bool
}

func (m *MockReaderForServiceValidator) Close() error {
	return nil
}

func (m *MockReaderForServiceValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	if m.serviceExists {
		service := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(service)
	}
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.hasBindings {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-binding"),
			},
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForServiceValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	if m.hasAliases {
		alias := models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
			},
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}
		return consume(alias)
	}
	return nil
}

func (m *MockReaderForServiceValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForServiceValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}
