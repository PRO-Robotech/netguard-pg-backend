package validation_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSvcSvcRuleValidator_ValidateExists tests the ValidateExists method
func TestSvcSvcRuleValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a SvcSvcRule for the test ID
	mockReader := &MockReaderForSvcSvcRuleValidator{
		ruleExists: true,
		ruleID:     "test-rule",
	}

	validator := validation.NewSvcSvcRuleValidator(mockReader)
	ruleID := models.NewResourceIdentifier("test-rule", models.WithNamespace("default"))

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

// TestSvcSvcRuleValidator_ValidateReferences tests the ValidateReferences method
func TestSvcSvcRuleValidator_ValidateReferences(t *testing.T) {
	tests := []struct {
		name       string
		rule       models.SvcSvcRule
		setupMocks func(reader *MockReaderForSvcSvcRuleValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid references - both services exist and ready",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: false,
		},
		{
			name: "ServiceFrom does not exist",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "non-existent-service",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = false
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "invalid serviceFrom reference",
		},
		{
			name: "ServiceFrom exists but not ready",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = false
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "serviceFrom is not ready yet",
		},
		{
			name: "ServiceTo does not exist",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "non-existent-service",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = false
			},
			wantErr: true,
			errMsg:  "invalid serviceTo reference",
		},
		{
			name: "ServiceTo exists but not ready",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = false
			},
			wantErr: true,
			errMsg:  "serviceTo is not ready yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForSvcSvcRuleValidator{}
			tt.setupMocks(reader)
			validator := validation.NewSvcSvcRuleValidator(reader)
			err := validator.ValidateReferences(context.Background(), tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReferences() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateReferences() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestSvcSvcRuleValidator_ValidateNoDuplicates tests the ValidateNoDuplicates method
func TestSvcSvcRuleValidator_ValidateNoDuplicates(t *testing.T) {
	tests := []struct {
		name       string
		rule       models.SvcSvcRule
		setupMocks func(reader *MockReaderForSvcSvcRuleValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "No duplicates",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.hasDuplicateRule = false
			},
			wantErr: false,
		},
		{
			name: "Duplicate rule exists with same service pair",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.hasDuplicateRule = true
				reader.duplicateRuleKey = "duplicate-rule"
				reader.duplicateServiceFromRef = netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				}
				reader.duplicateServiceToRef = netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				}
			},
			wantErr: true,
			errMsg:  "duplicate SvcSvcRule detected",
		},
		{
			name: "Error during duplicate check",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.listSvcSvcRulesError = fmt.Errorf("database error")
			},
			wantErr: true,
			errMsg:  "failed to check for duplicate SvcSvcRules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForSvcSvcRuleValidator{}
			tt.setupMocks(reader)
			validator := validation.NewSvcSvcRuleValidator(reader)
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

// TestSvcSvcRuleValidator_ValidatePriority tests the ValidatePriority method
func TestSvcSvcRuleValidator_ValidatePriority(t *testing.T) {
	tests := []struct {
		name     string
		priority int32
		wantErr  bool
	}{
		{"Valid priority - min", 0, false},
		{"Valid priority - mid", 500, false},
		{"Valid priority - max", 1000, false},
		{"Invalid priority - negative", -1, true},
		{"Invalid priority - too high", 1001, true},
	}

	validator := validation.NewSvcSvcRuleValidator(&MockReaderForSvcSvcRuleValidator{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := models.SvcSvcRule{
				Priority: tt.priority,
			}
			err := validator.ValidatePriority(rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePriority() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSvcSvcRuleValidator_ValidateForCreation tests the ValidateForCreation method
func TestSvcSvcRuleValidator_ValidateForCreation(t *testing.T) {
	tests := []struct {
		name       string
		rule       models.SvcSvcRule
		setupMocks func(reader *MockReaderForSvcSvcRuleValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid rule creation",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.ruleExists = false // Rule doesn't exist yet
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
				reader.hasDuplicateRule = false
			},
			wantErr: false,
		},
		{
			name: "Rule already exists",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("existing-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.ruleExists = true
				reader.ruleID = "existing-rule"
			},
			wantErr: true,
			errMsg:  "already exists",
		},
		{
			name: "Invalid priority",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 1001,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.ruleExists = false
			},
			wantErr: true,
			errMsg:  "priority must be between 0 and 1000",
		},
		{
			name: "ServiceFrom not ready",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.ruleExists = false
				reader.serviceFromExists = true
				reader.serviceFromReady = false
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "serviceFrom is not ready yet",
		},
		{
			name: "Duplicate service pair",
			rule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.ruleExists = false
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
				reader.hasDuplicateRule = true
				reader.duplicateRuleKey = "duplicate-rule"
				reader.duplicateServiceFromRef = netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				}
				reader.duplicateServiceToRef = netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				}
			},
			wantErr: true,
			errMsg:  "duplicate SvcSvcRule detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForSvcSvcRuleValidator{}
			tt.setupMocks(reader)
			validator := validation.NewSvcSvcRuleValidator(reader)
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

// TestSvcSvcRuleValidator_ValidateForUpdate tests the ValidateForUpdate method
func TestSvcSvcRuleValidator_ValidateForUpdate(t *testing.T) {
	tests := []struct {
		name       string
		oldRule    models.SvcSvcRule
		newRule    models.SvcSvcRule
		setupMocks func(reader *MockReaderForSvcSvcRuleValidator)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid update - only priority changed",
			oldRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
				Meta: models.Meta{
					Conditions: []metav1.Condition{},
				},
			},
			newRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 200,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: false,
		},
		{
			name: "Invalid update - changed ServiceFrom reference (service not found)",
			oldRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			newRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "new-service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "not found", // Validation checks references first, service doesn't exist
		},
		{
			name: "Invalid update - changed ServiceTo reference (service not found)",
			oldRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			newRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "new-service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "not found", // Validation checks references first, service doesn't exist
		},
		{
			name: "Invalid priority in update",
			oldRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: 100,
			},
			newRule: models.SvcSvcRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-rule", models.WithNamespace("default")),
				},
				ServiceFromRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-from",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				ServiceToRef: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name:       "service-to",
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
					},
					Namespace: "default",
				},
				Priority: -1,
			},
			setupMocks: func(reader *MockReaderForSvcSvcRuleValidator) {
				reader.serviceFromExists = true
				reader.serviceFromReady = true
				reader.serviceToExists = true
				reader.serviceToReady = true
			},
			wantErr: true,
			errMsg:  "priority must be between 0 and 1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForSvcSvcRuleValidator{}
			tt.setupMocks(reader)
			validator := validation.NewSvcSvcRuleValidator(reader)
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

// TestSvcSvcRuleValidator_CheckDependencies tests the CheckDependencies method
func TestSvcSvcRuleValidator_CheckDependencies(t *testing.T) {
	// Currently SvcSvcRule has no dependent resources, so this should always return nil
	validator := validation.NewSvcSvcRuleValidator(&MockReaderForSvcSvcRuleValidator{})
	ruleID := models.NewResourceIdentifier("test-rule", models.WithNamespace("default"))

	err := validator.CheckDependencies(context.Background(), ruleID)
	if err != nil {
		t.Errorf("CheckDependencies() should return nil for SvcSvcRule, got %v", err)
	}
}

// MockReaderForSvcSvcRuleValidator is a specialized mock for testing SvcSvcRuleValidator
type MockReaderForSvcSvcRuleValidator struct {
	ruleExists        bool
	ruleID            string
	serviceFromExists bool
	serviceFromReady  bool
	serviceToExists   bool
	serviceToReady    bool

	// For ValidateNoDuplicates
	hasDuplicateRule        bool
	duplicateRuleKey        string
	duplicateServiceFromRef netguardv1beta1.NamespacedObjectReference
	duplicateServiceToRef   netguardv1beta1.NamespacedObjectReference
	listSvcSvcRulesError    error
}

func (m *MockReaderForSvcSvcRuleValidator) ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope ports.Scope) error {
	// Return error if configured
	if m.listSvcSvcRulesError != nil {
		return m.listSvcSvcRulesError
	}

	// Return the rule if it exists
	if m.ruleExists {
		rule := models.SvcSvcRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID, models.WithNamespace("default")),
			},
		}
		if err := consume(rule); err != nil {
			return err
		}
	}

	// Return a duplicate rule if configured
	if m.hasDuplicateRule {
		duplicateRule := models.SvcSvcRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.duplicateRuleKey, models.WithNamespace("default")),
			},
			ServiceFromRef: m.duplicateServiceFromRef,
			ServiceToRef:   m.duplicateServiceToRef,
		}
		if err := consume(duplicateRule); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	if m.ruleExists && id.Name == m.ruleID {
		return &models.SvcSvcRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
		}, nil
	}
	return nil, fmt.Errorf("svcsvc rule not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	// Check ServiceFrom (handle both with and without namespace in name)
	if m.serviceFromExists && (id.Name == "service-from" || id.Key() == "default/service-from") {
		svc := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
			Meta: models.Meta{
				Conditions: []metav1.Condition{},
			},
		}
		if m.serviceFromReady {
			svc.Meta.Conditions = append(svc.Meta.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			})
		}
		return svc, nil
	}

	// Check ServiceTo
	if m.serviceToExists && (id.Name == "service-to" || id.Key() == "default/service-to") {
		svc := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
			Meta: models.Meta{
				Conditions: []metav1.Condition{},
			},
		}
		if m.serviceToReady {
			svc.Meta.Conditions = append(svc.Meta.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			})
		}
		return svc, nil
	}

	return nil, validation.NewEntityNotFoundError("service", id.Key())
}

// Stub implementations for other ports.Reader methods
func (m *MockReaderForSvcSvcRuleValidator) Close() error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	// Return ServiceFrom if it exists
	if m.serviceFromExists {
		svc := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("service-from", models.WithNamespace("default")),
			},
			Meta: models.Meta{
				Conditions: []metav1.Condition{},
			},
		}
		if m.serviceFromReady {
			svc.Meta.Conditions = append(svc.Meta.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			})
		}
		if err := consume(svc); err != nil {
			return err
		}
	}

	// Return ServiceTo if it exists
	if m.serviceToExists {
		svc := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("service-to", models.WithNamespace("default")),
			},
			Meta: models.Meta{
				Conditions: []metav1.Condition{},
			},
		}
		if m.serviceToReady {
			svc.Meta.Conditions = append(svc.Meta.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			})
		}
		if err := consume(svc); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListHosts(ctx context.Context, consume func(models.Host) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return &models.SyncStatus{}, nil
}

func (m *MockReaderForSvcSvcRuleValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("address group binding policy not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error) {
	return nil, nil
}

func (m *MockReaderForSvcSvcRuleValidator) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nil, fmt.Errorf("network binding not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	return nil, fmt.Errorf("host not found")
}

func (m *MockReaderForSvcSvcRuleValidator) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	return nil, fmt.Errorf("host binding not found")
}

func (m *MockReaderForSvcSvcRuleValidator) ListSvcFqdnRules(ctx context.Context, consume func(models.SvcFqdnRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForSvcSvcRuleValidator) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	return nil, fmt.Errorf("svc fqdn rule not found")
}
