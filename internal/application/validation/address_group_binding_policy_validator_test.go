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

// TestAddressGroupBindingPolicyValidator_ValidateExists tests the ValidateExists method
func TestAddressGroupBindingPolicyValidator_ValidateExists(t *testing.T) {
	tests := []struct {
		name           string
		policyID       models.ResourceIdentifier
		setupMocks     func(reader *MockReaderForAddressGroupBindingPolicyValidator)
		wantErr        bool
		wantErrMessage string
	}{
		{
			name:     "Policy exists",
			policyID: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.policyExists = true
				reader.policyID = "test-policy"
				reader.policyNamespace = "default"
			},
			wantErr: false,
		},
		{
			name:     "Policy does not exist",
			policyID: models.NewResourceIdentifier("non-existent-policy", models.WithNamespace("default")),
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.policyExists = false
			},
			wantErr:        true,
			wantErrMessage: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForAddressGroupBindingPolicyValidator{}
			tt.setupMocks(reader)
			validator := validation.NewAddressGroupBindingPolicyValidator(reader)
			err := validator.ValidateExists(context.Background(), tt.policyID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExists() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMessage != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMessage) {
					t.Errorf("ValidateExists() error message = %v, want to contain %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
	}
}

// TestAddressGroupBindingPolicyValidator_ValidateReferences tests the ValidateReferences method
func TestAddressGroupBindingPolicyValidator_ValidateReferences(t *testing.T) {
	tests := []struct {
		name           string
		policy         models.AddressGroupBindingPolicy
		setupMocks     func(reader *MockReaderForAddressGroupBindingPolicyValidator)
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "Valid references",
			policy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr: false,
		},
		{
			name: "Invalid service reference",
			policy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = false
				reader.addressGroupExists = true
			},
			wantErr:        true,
			wantErrMessage: "invalid service reference",
		},
		{
			name: "Invalid address group reference",
			policy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = false
			},
			wantErr:        true,
			wantErrMessage: "invalid address group reference",
		},
		{
			name: "Namespace mismatch",
			policy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("other-namespace")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr:        true,
			wantErrMessage: "policy namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForAddressGroupBindingPolicyValidator{}
			tt.setupMocks(reader)
			validator := validation.NewAddressGroupBindingPolicyValidator(reader)
			err := validator.ValidateReferences(context.Background(), tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReferences() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMessage != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMessage) {
					t.Errorf("ValidateReferences() error message = %v, want to contain %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
	}
}

// TestAddressGroupBindingPolicyValidator_ValidateForCreation tests the ValidateForCreation method
func TestAddressGroupBindingPolicyValidator_ValidateForCreation(t *testing.T) {
	tests := []struct {
		name           string
		policy         *models.AddressGroupBindingPolicy
		setupMocks     func(reader *MockReaderForAddressGroupBindingPolicyValidator)
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "Valid policy",
			policy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
				reader.hasDuplicatePolicy = false
			},
			wantErr: false,
		},
		{
			name: "Invalid service reference",
			policy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = false
				reader.addressGroupExists = true
				reader.hasDuplicatePolicy = false
			},
			wantErr:        true,
			wantErrMessage: "invalid service reference",
		},
		{
			name: "Invalid address group reference",
			policy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("non-existent-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = false
				reader.hasDuplicatePolicy = false
			},
			wantErr:        true,
			wantErrMessage: "invalid address group reference",
		},
		{
			name: "Namespace mismatch",
			policy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("other-namespace")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
				reader.hasDuplicatePolicy = false
			},
			wantErr:        true,
			wantErrMessage: "policy namespace",
		},
		{
			name: "Duplicate policy",
			policy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
				reader.hasDuplicatePolicy = true
				reader.duplicatePolicyServiceRef = models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				}
				reader.duplicatePolicyAddressGroupRef = models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				}
			},
			wantErr:        true,
			wantErrMessage: "duplicate policy found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForAddressGroupBindingPolicyValidator{}
			tt.setupMocks(reader)
			validator := validation.NewAddressGroupBindingPolicyValidator(reader)
			err := validator.ValidateForCreation(context.Background(), tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForCreation() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMessage != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMessage) {
					t.Errorf("ValidateForCreation() error message = %v, want to contain %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
	}
}

// TestAddressGroupBindingPolicyValidator_ValidateForUpdate tests the ValidateForUpdate method
func TestAddressGroupBindingPolicyValidator_ValidateForUpdate(t *testing.T) {
	tests := []struct {
		name           string
		oldPolicy      models.AddressGroupBindingPolicy
		newPolicy      *models.AddressGroupBindingPolicy
		setupMocks     func(reader *MockReaderForAddressGroupBindingPolicyValidator)
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "Valid update",
			oldPolicy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			newPolicy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr: false,
		},
		{
			name: "Invalid references",
			oldPolicy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			newPolicy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("other-namespace")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr:        true,
			wantErrMessage: "policy namespace",
		},
		{
			name: "Changed service reference",
			oldPolicy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("old-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			newPolicy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("new-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr:        true,
			wantErrMessage: "cannot change service reference",
		},
		{
			name: "Changed address group reference",
			oldPolicy: models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("old-ag", models.WithNamespace("default")),
				},
			},
			newPolicy: &models.AddressGroupBindingPolicy{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
				},
				ServiceRef: models.ServiceRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace("default")),
				},
				AddressGroupRef: models.AddressGroupRef{
					ResourceIdentifier: models.NewResourceIdentifier("new-ag", models.WithNamespace("default")),
				},
			},
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.serviceExists = true
				reader.addressGroupExists = true
			},
			wantErr:        true,
			wantErrMessage: "cannot change address group reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForAddressGroupBindingPolicyValidator{}
			tt.setupMocks(reader)
			validator := validation.NewAddressGroupBindingPolicyValidator(reader)
			err := validator.ValidateForUpdate(context.Background(), tt.oldPolicy, tt.newPolicy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMessage != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMessage) {
					t.Errorf("ValidateForUpdate() error message = %v, want to contain %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
	}
}

// TestAddressGroupBindingPolicyValidator_CheckDependencies tests the CheckDependencies method
func TestAddressGroupBindingPolicyValidator_CheckDependencies(t *testing.T) {
	tests := []struct {
		name           string
		policyID       models.ResourceIdentifier
		setupMocks     func(reader *MockReaderForAddressGroupBindingPolicyValidator)
		wantErr        bool
		wantErrMessage string
	}{
		{
			name:     "No dependencies",
			policyID: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.policyExists = true
				reader.policyID = "test-policy"
				reader.policyNamespace = "default"
				reader.policyServiceRef = models.NewServiceRef("test-service")
				reader.policyAddressGroupRef = models.NewAddressGroupRef("test-ag")
				reader.hasBindings = false
			},
			wantErr: false,
		},
		{
			name:     "Has dependencies",
			policyID: models.NewResourceIdentifier("test-policy", models.WithNamespace("default")),
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.policyExists = true
				reader.policyID = "test-policy"
				reader.policyNamespace = "default"
				reader.policyServiceRef = models.NewServiceRef("test-service")
				reader.policyAddressGroupRef = models.NewAddressGroupRef("test-ag")
				reader.hasBindings = true
			},
			wantErr:        true,
			wantErrMessage: "it is referenced by address_group_binding",
		},
		{
			name:     "Policy not found",
			policyID: models.NewResourceIdentifier("non-existent-policy", models.WithNamespace("default")),
			setupMocks: func(reader *MockReaderForAddressGroupBindingPolicyValidator) {
				reader.policyExists = false
			},
			wantErr:        true,
			wantErrMessage: "failed to get policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &MockReaderForAddressGroupBindingPolicyValidator{}
			tt.setupMocks(reader)
			validator := validation.NewAddressGroupBindingPolicyValidator(reader)
			err := validator.CheckDependencies(context.Background(), tt.policyID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckDependencies() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMessage != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMessage) {
					t.Errorf("CheckDependencies() error message = %v, want to contain %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
	}
}

// MockReaderForAddressGroupBindingPolicyValidator is a specialized mock for testing AddressGroupBindingPolicyValidator
type MockReaderForAddressGroupBindingPolicyValidator struct {
	policyExists          bool
	policyID              string
	policyNamespace       string
	policyServiceRef      models.ServiceRef
	policyAddressGroupRef models.AddressGroupRef
	hasBindings           bool

	// Для тестирования ValidateReferences
	serviceExists      bool
	addressGroupExists bool

	// Для тестирования ValidateForCreation
	hasDuplicatePolicy             bool
	duplicatePolicyServiceRef      models.ServiceRef
	duplicatePolicyAddressGroupRef models.AddressGroupRef
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) Close() error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	if m.serviceExists && scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				service := models.Service{
					SelfRef: models.SelfRef{
						ResourceIdentifier: id,
					},
				}
				if err := consume(service); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	if m.addressGroupExists && scope != nil {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				addressGroup := models.AddressGroup{
					SelfRef: models.SelfRef{
						ResourceIdentifier: id,
					},
				}
				if err := consume(addressGroup); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.hasBindings {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace(m.policyNamespace)),
			},
			ServiceRef:      m.policyServiceRef,
			AddressGroupRef: m.policyAddressGroupRef,
		}
		if err := consume(binding); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if m.serviceExists {
		return &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
		}, nil
	}
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if m.addressGroupExists {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: id,
			},
		}, nil
	}
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, fmt.Errorf("rule s2s not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, fmt.Errorf("service alias not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	if m.policyExists {
		policy := models.AddressGroupBindingPolicy{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.policyID, models.WithNamespace(m.policyNamespace)),
			},
			ServiceRef:      m.policyServiceRef,
			AddressGroupRef: m.policyAddressGroupRef,
		}
		if err := consume(policy); err != nil {
			return err
		}
	}

	// Для тестирования дубликатов в ValidateForCreation
	if m.hasDuplicatePolicy {
		duplicatePolicy := models.AddressGroupBindingPolicy{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("duplicate-policy", models.WithNamespace(m.policyNamespace)),
			},
			ServiceRef:      m.duplicatePolicyServiceRef,
			AddressGroupRef: m.duplicatePolicyAddressGroupRef,
		}
		if err := consume(duplicatePolicy); err != nil {
			// Если ошибка "duplicate policy found", это ожидаемое поведение для теста
			if err.Error() == "duplicate policy found" {
				return err
			}
			return err
		}
	}

	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	if m.policyExists && id.Key() == fmt.Sprintf("%s/%s", m.policyNamespace, m.policyID) {
		return &models.AddressGroupBindingPolicy{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.policyID, models.WithNamespace(m.policyNamespace)),
			},
			ServiceRef:      m.policyServiceRef,
			AddressGroupRef: m.policyAddressGroupRef,
		}, nil
	}
	return nil, fmt.Errorf("policy not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return nil, fmt.Errorf("IEAgAgRule not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForAddressGroupBindingPolicyValidator) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nil, fmt.Errorf("network binding not found")
}
