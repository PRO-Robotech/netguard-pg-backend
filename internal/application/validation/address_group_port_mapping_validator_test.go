package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupPortMappingValidator_ValidateExists tests the ValidateExists method of AddressGroupPortMappingValidator
func TestAddressGroupPortMappingValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group port mapping for the test ID
	mockReader := &MockReaderForAddressGroupPortMappingValidator{
		mappingExists: true,
		mappingID:     "test-mapping",
	}

	validator := validation.NewAddressGroupPortMappingValidator(mockReader)
	mappingID := models.NewResourceIdentifier("test-mapping")

	// Test when mapping exists
	err := validator.ValidateExists(context.Background(), mappingID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when mapping does not exist
	mockReader.mappingExists = false
	err = validator.ValidateExists(context.Background(), mappingID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupPortMappingValidator_ValidateReferences tests the ValidateReferences method of AddressGroupPortMappingValidator
func TestAddressGroupPortMappingValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupPortMappingValidator{
		serviceExists: true,
		serviceID:     "test-service",
	}

	validator := validation.NewAddressGroupPortMappingValidator(mockReader)

	// Create a port mapping with a service reference
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	accessPorts[models.ServiceRef{
		ResourceIdentifier: models.NewResourceIdentifier("test-service"),
	}] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	mapping := models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
		},
		AccessPorts: accessPorts,
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), mapping)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), mapping)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// MockReaderForAddressGroupPortMappingValidator is a specialized mock for testing AddressGroupPortMappingValidator
type MockReaderForAddressGroupPortMappingValidator struct {
	mappingExists bool
	mappingID     string
	serviceExists bool
	serviceID     string
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	if m.mappingExists {
		// Create a port mapping with a service reference
		accessPorts := make(map[models.ServiceRef]models.ServicePorts)
		accessPorts[models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
		}] = models.ServicePorts{
			Ports: models.ProtocolPorts{
				models.TCP: []models.PortRange{{Start: 80, End: 80}},
			},
		}

		mapping := models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.mappingID),
			},
			AccessPorts: accessPorts,
		}
		return consume(mapping)
	}
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) Close() error {
	return nil
}
