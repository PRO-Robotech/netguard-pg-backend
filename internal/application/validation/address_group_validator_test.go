package validation_test

import (
	"context"
	"fmt"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupValidator_ValidateExists tests the ValidateExists method of AddressGroupValidator
func TestAddressGroupValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group for the test ID
	mockReader := &MockReaderForAddressGroupValidator{
		addressGroupExists: true,
		addressGroupID:     "test-address-group",
	}

	validator := validation.NewAddressGroupValidator(mockReader)
	addressGroupID := models.NewResourceIdentifier("test-address-group")

	// Test when address group exists
	err := validator.ValidateExists(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when address group does not exist
	mockReader.addressGroupExists = false
	err = validator.ValidateExists(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupValidator_CheckDependencies tests the CheckDependencies method of AddressGroupValidator
func TestAddressGroupValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForAddressGroupValidator{
		addressGroupID: "test-address-group",
		hasServiceRefs: false,
		hasBindingRefs: false,
	}

	validator := validation.NewAddressGroupValidator(mockReader)
	addressGroupID := models.NewResourceIdentifier("test-address-group")

	// Test when no dependencies exist
	err := validator.CheckDependencies(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service dependency exists
	mockReader.hasServiceRefs = true
	err = validator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for service dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Test when address group binding dependency exists
	mockReader.hasServiceRefs = false
	mockReader.hasBindingRefs = true
	err = validator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for address group binding dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}
}

// MockReaderForAddressGroupValidator is a specialized mock for testing AddressGroupValidator
type MockReaderForAddressGroupValidator struct {
	addressGroupExists bool
	addressGroupID     string
	hasServiceRefs     bool
	hasBindingRefs     bool
}

func (m *MockReaderForAddressGroupValidator) Close() error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	if m.hasServiceRefs {
		service := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-service"),
			},
			AddressGroups: []models.AddressGroupRef{
				{
					ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
				},
			},
		}
		return consume(service)
	}
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.hasBindingRefs {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-binding"),
			},
			AddressGroupRef: models.AddressGroupRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if m.addressGroupExists && id.Key() == m.addressGroupID {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}, nil
	}
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForAddressGroupValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, fmt.Errorf("rule s2s not found")
}

func (m *MockReaderForAddressGroupValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, fmt.Errorf("service alias not found")
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("address group binding policy not found")
}

func (m *MockReaderForAddressGroupValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	// Since this mock is for AddressGroupValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("IEAgAgRule not found")
}
