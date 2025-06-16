package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestRuleS2SValidator_ValidateExists tests the ValidateExists method of RuleS2SValidator
func TestRuleS2SValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a rule s2s for the test ID
	mockReader := &MockReaderForRuleS2SValidator{
		ruleExists: true,
		ruleID:     "test-rule",
	}

	validator := validation.NewRuleS2SValidator(mockReader)
	ruleID := models.NewResourceIdentifier("test-rule")

	// Test when rule exists
	err := validator.ValidateExists(context.Background(), ruleID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when rule does not exist
	mockReader.ruleExists = false
	err = validator.ValidateExists(context.Background(), ruleID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestRuleS2SValidator_ValidateReferences tests the ValidateReferences method of RuleS2SValidator
func TestRuleS2SValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForRuleS2SValidator{
		serviceLocalAliasExists: true,
		serviceLocalAliasID:     "test-local-alias",
		serviceAliasExists:      true,
		serviceAliasID:          "test-alias",
	}

	validator := validation.NewRuleS2SValidator(mockReader)
	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
		},
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-local-alias"),
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
		},
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), rule)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service local reference is invalid
	mockReader.serviceLocalAliasExists = false
	err = validator.ValidateReferences(context.Background(), rule)
	if err == nil {
		t.Error("Expected error for invalid service local reference, got nil")
	}

	// Test when service reference is invalid
	mockReader.serviceLocalAliasExists = true
	mockReader.serviceAliasExists = false
	err = validator.ValidateReferences(context.Background(), rule)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// MockReaderForRuleS2SValidator is a specialized mock for testing RuleS2SValidator
type MockReaderForRuleS2SValidator struct {
	ruleExists             bool
	ruleID                 string
	serviceLocalAliasExists bool
	serviceLocalAliasID     string
	serviceAliasExists      bool
	serviceAliasID          string
}

func (m *MockReaderForRuleS2SValidator) Close() error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	if m.ruleExists {
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID),
			},
		}
		return consume(rule)
	}
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	// Check if we're looking for the service local alias
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if id.Key() == m.serviceLocalAliasID && m.serviceLocalAliasExists {
					alias := models.ServiceAlias{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.NewResourceIdentifier(m.serviceLocalAliasID),
						},
					}
					return consume(alias)
				}
				if id.Key() == m.serviceAliasID && m.serviceAliasExists {
					alias := models.ServiceAlias{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.NewResourceIdentifier(m.serviceAliasID),
						},
					}
					return consume(alias)
				}
			}
		}
	}
	return nil
}

func (m *MockReaderForRuleS2SValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForRuleS2SValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}