package validation_test

import (
	"context"
	"fmt"
	"strings"
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

// TestRuleS2SValidator_ValidateNoDuplicates tests the ValidateNoDuplicates method of RuleS2SValidator
func TestRuleS2SValidator_ValidateNoDuplicates(t *testing.T) {
	tests := []struct {
		name       string
		rule       models.RuleS2S
		setupMocks func(reader *MockReaderForRuleS2SValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "No duplicates",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias"),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.hasDuplicateRule = false
			},
			wantErr: false,
		},
		{
			name: "Duplicate rule exists",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias"),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.hasDuplicateRule = true
				reader.duplicateRuleKey = "duplicate-rule"
				reader.duplicateRuleTraffic = models.Traffic("ingress")
				reader.duplicateServiceLocalRef = models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias"),
				}
				reader.duplicateServiceRef = models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
				}
			},
			wantErr: true,
			errMsg:  "duplicate RuleS2S detected",
		},
		{
			name: "Error during duplicate check",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias"),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias"),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.listRuleS2SError = fmt.Errorf("database error")
			},
			wantErr: true,
			errMsg:  "failed to check for duplicate rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForRuleS2SValidator{}
			tt.setupMocks(reader)
			validator := validation.NewRuleS2SValidator(reader)
			err := validator.ValidateNoDuplicates(context.Background(), tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoDuplicates() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateNoDuplicates() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestRuleS2SValidator_ValidateForCreation tests the ValidateForCreation method of RuleS2SValidator
func TestRuleS2SValidator_ValidateForCreation(t *testing.T) {
	tests := []struct {
		name       string
		rule       models.RuleS2S
		setupMocks func(reader *MockReaderForRuleS2SValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid rule",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: false,
		},
		{
			name: "Invalid namespace rules",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("other-namespace")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "serviceLocalRef must be in the same namespace as the rule",
		},
		{
			name: "Invalid service local reference",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = false
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "invalid service local reference",
		},
		{
			name: "Invalid service reference",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = false
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "invalid service reference",
		},
		{
			name: "Duplicate rule",
			rule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = true
				reader.duplicateRuleKey = "duplicate-rule"
				reader.duplicateRuleTraffic = models.Traffic("ingress")
				reader.duplicateServiceLocalRef = models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				}
				reader.duplicateServiceRef = models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				}
			},
			wantErr: true,
			errMsg:  "duplicate RuleS2S detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForRuleS2SValidator{}
			tt.setupMocks(reader)
			validator := validation.NewRuleS2SValidator(reader)
			err := validator.ValidateForCreation(context.Background(), tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForCreation() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateForCreation() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestRuleS2SValidator_ValidateForUpdate tests the ValidateForUpdate method of RuleS2SValidator
func TestRuleS2SValidator_ValidateForUpdate(t *testing.T) {
	tests := []struct {
		name       string
		oldRule    models.RuleS2S
		newRule    models.RuleS2S
		setupMocks func(reader *MockReaderForRuleS2SValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid update",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: false,
		},
		{
			name: "Invalid namespace rules",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("other-namespace")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "serviceLocalRef must be in the same namespace as the rule",
		},
		{
			name: "Invalid service reference",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = false
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "invalid service reference",
		},
		{
			name: "Changed traffic direction",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("egress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "cannot change traffic direction",
		},
		{
			name: "Changed service local reference",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("old-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("new-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "new-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "test-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "cannot change local service reference",
		},
		{
			name: "Changed service reference",
			oldRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("old-alias", models.WithNamespace("default")),
				},
			},
			newRule: models.RuleS2S{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				Traffic: models.Traffic("ingress"),
				ServiceLocalRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-local-alias", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceAliasRef{
					ResourceIdentifier: models.NewResourceIdentifier("new-alias", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForRuleS2SValidator) {
				reader.serviceLocalAliasExists = true
				reader.serviceLocalAliasID = "test-local-alias"
				reader.serviceAliasExists = true
				reader.serviceAliasID = "new-alias"
				reader.hasDuplicateRule = false
			},
			wantErr: true,
			errMsg:  "cannot change target service reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForRuleS2SValidator{}
			tt.setupMocks(reader)
			validator := validation.NewRuleS2SValidator(reader)
			err := validator.ValidateForUpdate(context.Background(), tt.oldRule, tt.newRule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateForUpdate() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
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

	// For ValidateNoDuplicates
	hasDuplicateRule         bool
	duplicateRuleKey         string
	duplicateRuleTraffic     models.Traffic
	duplicateServiceLocalRef models.ServiceAliasRef
	duplicateServiceRef      models.ServiceAliasRef
	listRuleS2SError         error
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
	// Return error if configured
	if m.listRuleS2SError != nil {
		return m.listRuleS2SError
	}

	// Return the rule if it exists
	if m.ruleExists {
		rule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID),
			},
		}
		if err := consume(rule); err != nil {
			return err
		}
	}

	// Return a duplicate rule if configured
	if m.hasDuplicateRule {
		duplicateRule := models.RuleS2S{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.duplicateRuleKey),
			},
			Traffic:         m.duplicateRuleTraffic,
			ServiceLocalRef: m.duplicateServiceLocalRef,
			ServiceRef:      m.duplicateServiceRef,
		}
		if err := consume(duplicateRule); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockReaderForRuleS2SValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	// Check if we're looking for specific service aliases
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// Check for service local alias
				if m.serviceLocalAliasExists && (id.Key() == m.serviceLocalAliasID ||
					(id.Name == m.serviceLocalAliasID && (id.Namespace == "" || id.Namespace == "default"))) {
					alias := models.ServiceAlias{
						SelfRef: models.SelfRef{
							ResourceIdentifier: id,
						},
					}
					if err := consume(alias); err != nil {
						return err
					}
				}

				// Check for service alias
				if m.serviceAliasExists && (id.Key() == m.serviceAliasID ||
					(id.Name == m.serviceAliasID && (id.Namespace == "" || id.Namespace == m.serviceAliasNamespace || m.serviceAliasNamespace == ""))) {
					namespace := id.Namespace
					if namespace == "" && m.serviceAliasNamespace != "" {
						namespace = m.serviceAliasNamespace
					}
					alias := models.ServiceAlias{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.ResourceIdentifier{
								Name:      id.Name,
								Namespace: namespace,
							},
						},
					}
					if err := consume(alias); err != nil {
						return err
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
	// Check for service local alias
	if m.serviceLocalAliasExists && (id.Key() == m.serviceLocalAliasID ||
		(id.Name == m.serviceLocalAliasID && (id.Namespace == "" || id.Namespace == "default"))) {
		return &models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
		}, nil
	}

	// Check for service alias
	if m.serviceAliasExists && (id.Key() == m.serviceAliasID ||
		(id.Name == m.serviceAliasID && (id.Namespace == "" || id.Namespace == m.serviceAliasNamespace || m.serviceAliasNamespace == ""))) {
		namespace := id.Namespace
		if namespace == "" && m.serviceAliasNamespace != "" {
			namespace = m.serviceAliasNamespace
		}
		return &models.ServiceAlias{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      id.Name,
					Namespace: namespace,
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("service alias with id %s not found", id.Key())
}

func (m *MockReaderForRuleS2SValidator) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForRuleS2SValidator) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForRuleS2SValidator) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nil, fmt.Errorf("network binding not found")
}
