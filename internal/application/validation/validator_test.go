package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// MockReader is a mock implementation of ports.Reader for testing
type MockReader struct{}

func (m *MockReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}

func (m *MockReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return nil, nil
}

func (m *MockReader) Close() error {
	return nil
}

func (m *MockReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}

// TestNewDependencyValidator tests that a new DependencyValidator can be created
func TestNewDependencyValidator(t *testing.T) {
	mockReader := &MockReader{}
	validator := validation.NewDependencyValidator(mockReader)

	if validator == nil {
		t.Error("Expected non-nil validator")
	}
}

// TestNewValidationError tests that a new ValidationError can be created
func TestNewValidationError(t *testing.T) {
	err := validation.NewValidationError("test error")

	if err == nil {
		t.Error("Expected non-nil error")
	}

	if err.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", err.Error())
	}
}

// TestNewEntityNotFoundError tests that a new EntityNotFoundError can be created
func TestNewEntityNotFoundError(t *testing.T) {
	err := validation.NewEntityNotFoundError("service", "test-id")

	if err == nil {
		t.Error("Expected non-nil error")
	}

	expectedMsg := "service with id test-id not found"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestNewDependencyExistsError tests that a new DependencyExistsError can be created
func TestNewDependencyExistsError(t *testing.T) {
	err := validation.NewDependencyExistsError("service", "test-id", "service_alias")

	if err == nil {
		t.Error("Expected non-nil error")
	}

	expectedMsg := "cannot delete service with id test-id: it is referenced by service_alias"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}
