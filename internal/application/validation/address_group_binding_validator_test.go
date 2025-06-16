package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupBindingValidator_ValidateExists tests the ValidateExists method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group binding for the test ID
	mockReader := &MockReaderForAddressGroupBindingValidator{
		bindingExists: true,
		bindingID:     "test-binding",
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)
	bindingID := models.NewResourceIdentifier("test-binding")

	// Test when binding exists
	err := validator.ValidateExists(context.Background(), bindingID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when binding does not exist
	mockReader.bindingExists = false
	err = validator.ValidateExists(context.Background(), bindingID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupBindingValidator_ValidateReferences tests the ValidateReferences method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupBindingValidator{
		serviceExists:     true,
		serviceID:         "test-service",
		addressGroupExists: true,
		addressGroupID:     "test-address-group",
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)
	binding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding"),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}

	// Test when address group reference is invalid
	mockReader.serviceExists = true
	mockReader.addressGroupExists = false
	err = validator.ValidateReferences(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for invalid address group reference, got nil")
	}
}

// MockReaderForAddressGroupBindingValidator is a specialized mock for testing AddressGroupBindingValidator
type MockReaderForAddressGroupBindingValidator struct {
	bindingExists      bool
	bindingID          string
	serviceExists      bool
	serviceID          string
	addressGroupExists bool
	addressGroupID     string
}

func (m *MockReaderForAddressGroupBindingValidator) Close() error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	if m.addressGroupExists {
		addressGroup := models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}
		return consume(addressGroup)
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.bindingExists {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.bindingID),
			},
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}