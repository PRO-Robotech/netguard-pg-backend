package validation_test

import (
	"context"
	"fmt"
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

// TestRuleS2SValidator_ValidateNamespaceRules tests the ValidateNamespaceRules method of RuleS2SValidator
func TestRuleS2SValidator_ValidateNamespaceRules(t *testing.T) {
	// Test case 1: ServiceLocalRef in different namespace than rule
	t.Run("ServiceLocalRef in different namespace", func(t *testing.T) {
		mockReader := &MockReaderForRuleS2SValidator{
			serviceLocalAliasExists: true,
			serviceLocalAliasID:     "test-local-alias",
			serviceAliasExists:      true,
			serviceAliasID:          "test-alias",
		}

		validator := validation.NewRuleS2SValidator(mockReader)
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("namespace1")),
			},
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("namespace2")),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
			},
		}

		err := validator.ValidateNamespaceRules(context.Background(), rule)
		if err == nil {
			t.Error("Expected error for ServiceLocalRef in different namespace, got nil")
		}
	})

	// Test case 2: ServiceRef with no namespace, but rule has namespace
	t.Run("ServiceRef with no namespace", func(t *testing.T) {
		// Case 2.1: ServiceRef exists in rule's namespace
		mockReader := &MockReaderForRuleS2SValidator{
			serviceLocalAliasExists: true,
			serviceLocalAliasID:     "test-local-alias",
			serviceAliasExists:      true,
			serviceAliasID:          "test-alias",
			serviceAliasNamespace:   "namespace1", // Same as rule's namespace
		}

		validator := validation.NewRuleS2SValidator(mockReader)
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("namespace1")),
			},
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("namespace1")),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-alias"), // No namespace
			},
		}

		err := validator.ValidateNamespaceRules(context.Background(), rule)
		if err != nil {
			t.Errorf("Expected no error when ServiceRef exists in rule's namespace, got %v", err)
		}

		// Case 2.2: ServiceRef does not exist in rule's namespace
		mockReader.serviceAliasExists = false
		err = validator.ValidateNamespaceRules(context.Background(), rule)
		if err == nil {
			t.Error("Expected error when ServiceRef does not exist in rule's namespace, got nil")
		}
	})

	// Test case 3: Valid namespaces
	t.Run("Valid namespaces", func(t *testing.T) {
		mockReader := &MockReaderForRuleS2SValidator{
			serviceLocalAliasExists: true,
			serviceLocalAliasID:     "test-local-alias",
			serviceAliasExists:      true,
			serviceAliasID:          "test-alias",
		}

		validator := validation.NewRuleS2SValidator(mockReader)
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("namespace1")),
			},
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("namespace1")),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("namespace2")),
			},
		}

		err := validator.ValidateNamespaceRules(context.Background(), rule)
		if err != nil {
			t.Errorf("Expected no error for valid namespaces, got %v", err)
		}
	})
}

// MockReaderForRuleS2SValidator is a specialized mock for testing RuleS2SValidator
type MockReaderForRuleS2SValidator struct {
	ruleExists              bool
	ruleID                  string
	serviceLocalAliasExists bool
	serviceLocalAliasID     string
	serviceAliasExists      bool
	serviceAliasID          string
	serviceAliasNamespace   string
}

func (m *MockReaderForRuleS2SValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("address group binding policy not found")
}

func (m *MockReaderForRuleS2SValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	// Since this mock is for RuleS2SValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("IEAgAgRule not found")
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
				// Check for service local alias
				if id.Key() == m.serviceLocalAliasID && m.serviceLocalAliasExists {
					alias := models.ServiceAlias{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.NewResourceIdentifier(m.serviceLocalAliasID),
						},
					}
					return consume(alias)
				}

				// Check for service alias
				if m.serviceAliasExists {
					// If namespace is specified in the ID, check it matches
					if id.Namespace != "" && m.serviceAliasNamespace != "" && id.Namespace != m.serviceAliasNamespace {
						continue
					}

					// Check if the name matches
					if id.Name == m.serviceAliasID {
						alias := models.ServiceAlias{
							SelfRef: models.SelfRef{
								ResourceIdentifier: models.ResourceIdentifier{
									Name:      m.serviceAliasID,
									Namespace: m.serviceAliasNamespace,
								},
							},
						}
						return consume(alias)
					}
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
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForRuleS2SValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForRuleS2SValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	if m.ruleExists && id.Key() == m.ruleID {
		return &models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID),
			},
		}, nil
	}
	return nil, fmt.Errorf("rule s2s not found")
}

func (m *MockReaderForRuleS2SValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	if (m.serviceLocalAliasExists && id.Key() == m.serviceLocalAliasID) ||
		(m.serviceAliasExists && id.Name == m.serviceAliasID) {
		return &models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      id.Name,
					Namespace: m.serviceAliasNamespace,
				},
			},
		}, nil
	}
	return nil, fmt.Errorf("service alias not found")
}
