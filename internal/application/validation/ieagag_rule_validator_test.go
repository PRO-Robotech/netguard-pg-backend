package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// MockReaderForIEAgAgRuleValidator is a specialized mock for testing IEAgAgRuleValidator
type MockReaderForIEAgAgRuleValidator struct {
	ruleExists              bool
	ruleID                  string
	addressGroupLocalExists bool
	addressGroupLocalID     string
	addressGroupExists      bool
	addressGroupID          string
}

func (m *MockReaderForIEAgAgRuleValidator) Close() error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	// If we have a scope with resource identifiers, check if any of them match our address groups
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// Check for local address group
				if id.Key() == m.addressGroupLocalID && m.addressGroupLocalExists {
					ag := models.AddressGroup{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupLocalID),
						},
					}
					if err := consume(ag); err != nil {
						return err
					}
				}

				// Check for target address group
				if id.Key() == m.addressGroupID && m.addressGroupExists {
					ag := models.AddressGroup{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
						},
					}
					if err := consume(ag); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// If no scope or not a resource identifier scope, return all address groups
	if m.addressGroupLocalExists {
		ag := models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupLocalID),
			},
		}
		if err := consume(ag); err != nil {
			return err
		}
	}

	if m.addressGroupExists {
		ag := models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}
		if err := consume(ag); err != nil {
			return err
		}
	}

	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	if m.ruleExists {
		rule := models.IEAgAgRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID),
			},
		}
		return consume(rule)
	}
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if id.Key() == m.addressGroupLocalID && m.addressGroupLocalExists {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupLocalID),
			},
		}, nil
	}
	if id.Key() == m.addressGroupID && m.addressGroupExists {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}, nil
	}
	return nil, validation.NewEntityNotFoundError("AddressGroup", id.Key())
}

func (m *MockReaderForIEAgAgRuleValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	if id.Key() == m.ruleID && m.ruleExists {
		return &models.IEAgAgRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.ruleID),
			},
		}, nil
	}
	return nil, validation.NewEntityNotFoundError("IEAgAgRule", id.Key())
}

func (m *MockReaderForIEAgAgRuleValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRuleValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}

// TestIEAgAgRuleValidator_ValidateExists tests the ValidateExists method of IEAgAgRuleValidator
func TestIEAgAgRuleValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns a rule for the test ID
	mockReader := &MockReaderForIEAgAgRuleValidator{
		ruleExists: true,
		ruleID:     "test-rule",
	}

	validator := validation.NewIEAgAgRuleValidator(mockReader)
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

// TestIEAgAgRuleValidator_ValidateReferences tests the ValidateReferences method of IEAgAgRuleValidator
func TestIEAgAgRuleValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForIEAgAgRuleValidator{
		addressGroupLocalExists: true,
		addressGroupLocalID:     "test-local-ag",
		addressGroupExists:      true,
		addressGroupID:          "test-ag",
	}

	validator := validation.NewIEAgAgRuleValidator(mockReader)
	rule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
		},
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-local-ag"),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-ag"),
		},
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), rule)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when address group local reference is invalid
	mockReader.addressGroupLocalExists = false
	err = validator.ValidateReferences(context.Background(), rule)
	if err == nil {
		t.Error("Expected error for invalid address group local reference, got nil")
	}

	// Test when address group reference is invalid
	mockReader.addressGroupLocalExists = true
	mockReader.addressGroupExists = false
	err = validator.ValidateReferences(context.Background(), rule)
	if err == nil {
		t.Error("Expected error for invalid address group reference, got nil")
	}
}

// TestIEAgAgRuleValidator_ValidatePortSpec tests the ValidatePortSpec method of IEAgAgRuleValidator
func TestIEAgAgRuleValidator_ValidatePortSpec(t *testing.T) {
	mockReader := &MockReaderForIEAgAgRuleValidator{}
	validator := validation.NewIEAgAgRuleValidator(mockReader)

	// Test valid port specs
	testCases := []struct {
		name     string
		portSpec models.PortSpec
		valid    bool
	}{
		{
			name: "Valid destination port only",
			portSpec: models.PortSpec{
				Destination: "80",
			},
			valid: true,
		},
		{
			name: "Valid source and destination ports",
			portSpec: models.PortSpec{
				Source:      "8080",
				Destination: "80",
			},
			valid: true,
		},
		{
			name: "Valid port range",
			portSpec: models.PortSpec{
				Destination: "80-90",
			},
			valid: true,
		},
		{
			name: "Invalid destination port (non-numeric)",
			portSpec: models.PortSpec{
				Destination: "abc",
			},
			valid: false,
		},
		{
			name: "Invalid source port (non-numeric)",
			portSpec: models.PortSpec{
				Source:      "abc",
				Destination: "80",
			},
			valid: false,
		},
		{
			name: "Invalid port range (invalid format)",
			portSpec: models.PortSpec{
				Destination: "90-80",
			},
			valid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidatePortSpec(context.Background(), tc.portSpec)
			if tc.valid && err != nil {
				t.Errorf("Expected no error for valid port spec, got %v", err)
			}
			if !tc.valid && err == nil {
				t.Error("Expected error for invalid port spec, got nil")
			}
		})
	}
}

// TestIEAgAgRuleValidator_ValidateForCreation tests the ValidateForCreation method of IEAgAgRuleValidator
func TestIEAgAgRuleValidator_ValidateForCreation(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForIEAgAgRuleValidator{
		addressGroupLocalExists: true,
		addressGroupLocalID:     "test-local-ag",
		addressGroupExists:      true,
		addressGroupID:          "test-ag",
	}

	validator := validation.NewIEAgAgRuleValidator(mockReader)

	// Test valid rule
	validRule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
		},
		Transport: models.TCP,
		Traffic:   models.INGRESS,
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-local-ag"),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-ag"),
		},
		Ports: []models.PortSpec{
			{
				Destination: "80",
			},
		},
		Action:   models.ActionAccept,
		Logs:     true,
		Priority: 100,
	}

	err := validator.ValidateForCreation(context.Background(), validRule)
	if err != nil {
		t.Errorf("Expected no error for valid rule, got %v", err)
	}

	// Test invalid references
	mockReader.addressGroupLocalExists = false
	err = validator.ValidateForCreation(context.Background(), validRule)
	if err == nil {
		t.Error("Expected error for invalid address group local reference, got nil")
	}

	// Test invalid port spec
	mockReader.addressGroupLocalExists = true
	invalidPortRule := validRule
	invalidPortRule.Ports = []models.PortSpec{
		{
			Destination: "abc",
		},
	}
	err = validator.ValidateForCreation(context.Background(), invalidPortRule)
	if err == nil {
		t.Error("Expected error for invalid port spec, got nil")
	}
}

// TestIEAgAgRuleValidator_ValidateForUpdate tests the ValidateForUpdate method of IEAgAgRuleValidator
func TestIEAgAgRuleValidator_ValidateForUpdate(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForIEAgAgRuleValidator{
		addressGroupLocalExists: true,
		addressGroupLocalID:     "test-local-ag",
		addressGroupExists:      true,
		addressGroupID:          "test-ag",
	}

	validator := validation.NewIEAgAgRuleValidator(mockReader)

	// Create old and new rules
	oldRule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-rule"),
		},
		Transport: models.TCP,
		Traffic:   models.INGRESS,
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-local-ag"),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-ag"),
		},
		Ports: []models.PortSpec{
			{
				Destination: "80",
			},
		},
		Action:   models.ActionAccept,
		Logs:     true,
		Priority: 100,
	}

	// Test valid update (only changing ports, logs, action, priority)
	validNewRule := oldRule
	validNewRule.Ports = []models.PortSpec{
		{
			Destination: "443",
		},
	}
	validNewRule.Logs = false
	validNewRule.Action = models.ActionDrop
	validNewRule.Priority = 200

	err := validator.ValidateForUpdate(context.Background(), oldRule, validNewRule)
	if err != nil {
		t.Errorf("Expected no error for valid update, got %v", err)
	}

	// Test invalid update (changing transport)
	invalidTransportRule := oldRule
	invalidTransportRule.Transport = models.UDP
	err = validator.ValidateForUpdate(context.Background(), oldRule, invalidTransportRule)
	if err == nil {
		t.Error("Expected error for changing transport, got nil")
	}

	// Test invalid update (changing traffic)
	invalidTrafficRule := oldRule
	invalidTrafficRule.Traffic = models.EGRESS
	err = validator.ValidateForUpdate(context.Background(), oldRule, invalidTrafficRule)
	if err == nil {
		t.Error("Expected error for changing traffic, got nil")
	}

	// Test invalid update (changing address group local)
	invalidAddressGroupLocalRule := oldRule
	invalidAddressGroupLocalRule.AddressGroupLocal = models.AddressGroupRef{
		ResourceIdentifier: models.NewResourceIdentifier("different-local-ag"),
	}
	err = validator.ValidateForUpdate(context.Background(), oldRule, invalidAddressGroupLocalRule)
	if err == nil {
		t.Error("Expected error for changing address group local, got nil")
	}

	// Test invalid update (changing address group)
	invalidAddressGroupRule := oldRule
	invalidAddressGroupRule.AddressGroup = models.AddressGroupRef{
		ResourceIdentifier: models.NewResourceIdentifier("different-ag"),
	}
	err = validator.ValidateForUpdate(context.Background(), oldRule, invalidAddressGroupRule)
	if err == nil {
		t.Error("Expected error for changing address group, got nil")
	}

	// Test invalid port spec
	invalidPortRule := oldRule
	invalidPortRule.Ports = []models.PortSpec{
		{
			Destination: "abc",
		},
	}
	err = validator.ValidateForUpdate(context.Background(), oldRule, invalidPortRule)
	if err == nil {
		t.Error("Expected error for invalid port spec, got nil")
	}
}
