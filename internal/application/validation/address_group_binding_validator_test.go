package validation_test

import (
	"context"
	"fmt"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupBindingValidator_ValidateForCreation tests the ValidateForCreation method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateForCreation(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupBindingValidator{
		serviceExists:      true,
		serviceID:          "test-service",
		serviceNamespace:   "test-ns",
		addressGroupExists: true,
		addressGroupID:     "test-address-group",
		portMappingExists:  false,
		portMappingID:      "test-address-group", // Same as addressGroupID
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)
	binding := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	// Test when port mapping doesn't exist (should pass validation)
	err := validator.ValidateForCreation(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when port mapping exists with non-overlapping ports (should pass validation)
	mockReader.portMappingExists = true
	mockReader.useOverlappingPorts = false

	err = validator.ValidateForCreation(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when port mapping exists with overlapping ports (should fail validation)
	mockReader.useOverlappingPorts = true

	err = validator.ValidateForCreation(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for overlapping ports, got nil")
	}

	// Test with namespace mismatch
	binding.Namespace = "different-ns"
	err = validator.ValidateForCreation(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for namespace mismatch, got nil")
	}

	// Test with non-existent service
	mockReader.serviceExists = false
	binding.Namespace = "test-ns" // Reset namespace
	err = validator.ValidateForCreation(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for non-existent service, got nil")
	}

	// Test with non-existent address group
	mockReader.serviceExists = true
	mockReader.addressGroupExists = false
	err = validator.ValidateForCreation(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for non-existent address group, got nil")
	}
}

// TestAddressGroupBindingValidator_ValidateExists tests the ValidateExists method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group binding for the test ID
	mockReader := &MockReaderForAddressGroupBindingValidator{
		bindingExists: true,
		bindingID:     "test-binding",
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)
	bindingID := models.NewResourceIdentifier("test-binding")

	// Test when binding exists
	err := validator.ValidateExists(context.Background(), bindingID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when binding does not exist
	mockReader.bindingExists = false
	err = validator.ValidateExists(context.Background(), bindingID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupBindingValidator_ValidateForUpdate tests the ValidateForUpdate method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateForUpdate(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupBindingValidator{
		serviceExists:      true,
		serviceID:          "test-service",
		serviceNamespace:   "test-ns",
		addressGroupExists: true,
		addressGroupID:     "test-address-group",
		portMappingExists:  false,
		portMappingID:      "test-address-group", // Same as addressGroupID
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)

	// Create old binding
	oldBinding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	// Create new binding with same references
	newBinding := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	// Test when port mapping doesn't exist (should pass validation)
	err := validator.ValidateForUpdate(context.Background(), oldBinding, newBinding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when port mapping exists with non-overlapping ports (should pass validation)
	mockReader.portMappingExists = true
	mockReader.useOverlappingPorts = false

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBinding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when port mapping exists with overlapping ports (should fail validation)
	mockReader.useOverlappingPorts = true

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBinding)
	if err == nil {
		t.Error("Expected error for overlapping ports, got nil")
	}

	// Test with changed service reference (should return error)
	newBindingWithChangedService := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("different-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBindingWithChangedService)
	if err == nil {
		t.Error("Expected error when service reference is changed, got nil")
	}

	// Test when address group reference is changed (should return error)
	newBindingWithChangedAddressGroup := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("other-address-group"),
		},
	}

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBindingWithChangedAddressGroup)
	if err == nil {
		t.Error("Expected error when changing address group reference, got nil")
	}
}

// TestAddressGroupBindingValidator_ValidateReferences tests the ValidateReferences method of AddressGroupBindingValidator
func TestAddressGroupBindingValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupBindingValidator{
		serviceExists:      true,
		serviceID:          "test-service",
		serviceNamespace:   "test-ns",
		addressGroupExists: true,
		addressGroupID:     "test-address-group",
	}

	validator := validation.NewAddressGroupBindingValidator(mockReader)
	binding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	// Test when all references are valid and namespace matches
	err := validator.ValidateReferences(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when namespace doesn't match
	bindingWithMismatchedNS := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("other-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}
	err = validator.ValidateReferences(context.Background(), bindingWithMismatchedNS)
	if err == nil {
		t.Error("Expected error for mismatched namespaces, got nil")
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}

	// Test when address group reference is invalid
	mockReader.serviceExists = true
	mockReader.addressGroupExists = false
	err = validator.ValidateReferences(context.Background(), binding)
	if err == nil {
		t.Error("Expected error for invalid address group reference, got nil")
	}
}

// MockReaderForAddressGroupBindingValidator is a specialized mock for testing AddressGroupBindingValidator
type MockReaderForAddressGroupBindingValidator struct {
	bindingExists       bool
	bindingID           string
	serviceExists       bool
	serviceID           string
	serviceNamespace    string
	addressGroupExists  bool
	addressGroupID      string
	portMappingExists   bool
	portMappingID       string
	useOverlappingPorts bool
}

func (m *MockReaderForAddressGroupBindingValidator) Close() error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.bindingExists {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.bindingID),
			},
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if m.serviceExists && id.Key() == m.serviceID {
		return &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID, models.WithNamespace(m.serviceNamespace)),
			},
			IngressPorts: []models.IngressPort{
				{
					Protocol: models.TCP,
					Port:     "80",
				},
				{
					Protocol: models.UDP,
					Port:     "53",
				},
			},
		}, nil
	}
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if m.addressGroupExists && id.Key() == m.addressGroupID {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID, models.WithNamespace(m.serviceNamespace)),
			},
		}, nil
	}
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	if m.bindingExists && id.Key() == m.bindingID {
		return &models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.bindingID, models.WithNamespace(m.serviceNamespace)),
			},
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
			AddressGroupRef: models.AddressGroupRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupID),
			},
		}, nil
	}
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	if m.portMappingExists && id.Key() == m.portMappingID {
		// Create a port mapping with some existing ports
		portMapping := &models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.portMappingID),
			},
			AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
		}

		// Add some existing service ports for a different service
		// Use a service name that is different from the one we're testing with
		var existingServiceRef models.ServiceRef
		if m.useOverlappingPorts {
			// Use a different service name but with the same port to cause an overlap
			existingServiceRef = models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier("different-service"),
			}
		} else {
			// Use a different service name to avoid overlap
			existingServiceRef = models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier("different-service"),
			}
		}

		existingServicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		if m.useOverlappingPorts {
			// Add TCP port 80 (same as the test service's port 80) - this will cause an overlap
			existingServicePorts.Ports[models.TCP] = append(
				existingServicePorts.Ports[models.TCP],
				models.PortRange{Start: 80, End: 80},
			)
		} else {
			// Add TCP port 8080 (different from the test service's port 80)
			existingServicePorts.Ports[models.TCP] = append(
				existingServicePorts.Ports[models.TCP],
				models.PortRange{Start: 8080, End: 8080},
			)
		}

		// Add UDP port range 1000-2000 (different from the test service's port 53)
		existingServicePorts.Ports[models.UDP] = append(
			existingServicePorts.Ports[models.UDP],
			models.PortRange{Start: 1000, End: 2000},
		)

		portMapping.AccessPorts[existingServiceRef] = existingServicePorts

		return portMapping, nil
	}
	return nil, fmt.Errorf("port mapping not found")
}

func (m *MockReaderForAddressGroupBindingValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	// Since this mock is for AddressGroupBindingValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("rule s2s not found")
}

func (m *MockReaderForAddressGroupBindingValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	// Since this mock is for AddressGroupBindingValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("service alias not found")
}

func (m *MockReaderForAddressGroupBindingValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	// Since this mock is for AddressGroupBindingValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("IEAgAgRule not found")
}
