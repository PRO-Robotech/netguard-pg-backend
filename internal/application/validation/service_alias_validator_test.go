package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestServiceAliasValidator_ValidateExists tests the ValidateExists method of ServiceAliasValidator
func TestServiceAliasValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a service alias for the test ID
	mockReader := &MockReaderForServiceAliasValidator{
		aliasExists: true,
		aliasID:     "test-alias",
	}

	validator := validation.NewServiceAliasValidator(mockReader)
	aliasID := models.NewResourceIdentifier("test-alias")

	// Test when alias exists
	err := validator.ValidateExists(context.Background(), aliasID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when alias does not exist
	mockReader.aliasExists = false
	err = validator.ValidateExists(context.Background(), aliasID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestServiceAliasValidator_ValidateReferences tests the ValidateReferences method of ServiceAliasValidator
func TestServiceAliasValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForServiceAliasValidator{
		serviceExists: true,
		serviceID:     "test-service",
	}

	validator := validation.NewServiceAliasValidator(mockReader)
	alias := models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), alias)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), alias)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestServiceAliasValidator_CheckDependencies tests the CheckDependencies method of ServiceAliasValidator
func TestServiceAliasValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForServiceAliasValidator{
		aliasID:   "test-alias",
		hasRuleRefs: false,
	}

	validator := validation.NewServiceAliasValidator(mockReader)
	aliasID := models.NewResourceIdentifier("test-alias")

	// Test when no dependencies exist
	err := validator.CheckDependencies(context.Background(), aliasID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when rule s2s dependency exists
	mockReader.hasRuleRefs = true
	err = validator.CheckDependencies(context.Background(), aliasID)
	if err == nil {
		t.Error("Expected error for rule s2s dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}
}

// MockReaderForServiceAliasValidator is a specialized mock for testing ServiceAliasValidator
type MockReaderForServiceAliasValidator struct {
	aliasExists   bool
	aliasID       string
	serviceExists bool
	serviceID     string
	hasRuleRefs   bool
}

func (m *MockReaderForServiceAliasValidator) Close() error {
	return nil
}

func (m *MockReaderForServiceAliasValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
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

func (m *MockReaderForServiceAliasValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceAliasValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceAliasValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceAliasValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	if m.hasRuleRefs {
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
			},
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.aliasID),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("other-alias"),
			},
		}
		return consume(rule)
	}
	return nil
}

func (m *MockReaderForServiceAliasValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	if m.aliasExists {
		alias := models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.aliasID),
			},
		}
		return consume(alias)
	}
	return nil
}

func (m *MockReaderForServiceAliasValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForServiceAliasValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}