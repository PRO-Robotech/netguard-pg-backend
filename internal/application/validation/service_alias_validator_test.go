package validation_test

import (
	"context"
	"fmt"
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
		serviceExists:    true,
		serviceID:        "test-service",
		serviceNamespace: "test-ns",
	}

	validator := validation.NewServiceAliasValidator(mockReader)

	// Test when all references are valid and namespace matches
	alias := models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	err := validator.ValidateReferences(context.Background(), alias)
	if err != nil {
		t.Errorf("Expected no error for matching namespaces, got %v", err)
	}

	// Test when namespace doesn't match
	aliasMismatchedNS := models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("other-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	err = validator.ValidateReferences(context.Background(), aliasMismatchedNS)
	if err == nil {
		t.Error("Expected error for mismatched namespaces, got nil")
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), alias)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestServiceAliasValidator_ValidateForCreation tests the ValidateForCreation method of ServiceAliasValidator
func TestServiceAliasValidator_ValidateForCreation(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForServiceAliasValidator{
		serviceExists:    true,
		serviceID:        "test-service",
		serviceNamespace: "test-ns",
	}

	validator := validation.NewServiceAliasValidator(mockReader)

	// Test when namespace is not specified (should be auto-filled)
	aliasWithoutNS := &models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	err := validator.ValidateForCreation(context.Background(), aliasWithoutNS)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that namespace was set from service
	if aliasWithoutNS.Namespace != "test-ns" {
		t.Errorf("Expected namespace to be 'test-ns', got '%s'", aliasWithoutNS.Namespace)
	}

	// Test when namespace is specified and matches service namespace
	aliasWithMatchingNS := &models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	err = validator.ValidateForCreation(context.Background(), aliasWithMatchingNS)
	if err != nil {
		t.Errorf("Expected no error for matching namespaces, got %v", err)
	}

	// Test when namespace is specified but doesn't match service namespace
	aliasWithMismatchedNS := &models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("other-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
	}

	err = validator.ValidateForCreation(context.Background(), aliasWithMismatchedNS)
	if err == nil {
		t.Error("Expected error for mismatched namespaces, got nil")
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateForCreation(context.Background(), aliasWithoutNS)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestServiceAliasValidator_CheckDependencies tests the CheckDependencies method of ServiceAliasValidator
func TestServiceAliasValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForServiceAliasValidator{
		aliasID:     "test-alias",
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
	aliasExists      bool
	aliasID          string
	serviceExists    bool
	serviceID        string
	serviceNamespace string
	hasRuleRefs      bool
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
	if m.serviceExists && id.Key() == m.serviceID {
		return &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID, models.WithNamespace(m.serviceNamespace)),
			},
		}, nil
	}
	return nil, fmt.Errorf("service not found")
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

func (m *MockReaderForServiceAliasValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForServiceAliasValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}
