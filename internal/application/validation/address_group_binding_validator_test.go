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

	// Test when port mapping doesn't exist (should create a new one)
	err := validator.ValidateForCreation(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that a port mapping was created
	if binding.PortMapping == nil {
		t.Error("Expected port mapping to be created, got nil")
	}

	// Check that the port mapping has the correct ID
	if binding.PortMapping.ResourceIdentifier.Key() != "test-address-group" {
		t.Errorf("Expected port mapping ID to be 'test-address-group', got '%s'", binding.PortMapping.ResourceIdentifier.Key())
	}

	// Check that the port mapping has the service ports
	serviceRef := models.ServiceRef{ResourceIdentifier: models.NewResourceIdentifier("test-service")}
	if _, ok := binding.PortMapping.AccessPorts[serviceRef]; !ok {
		t.Error("Expected port mapping to have service ports, got none")
	}

	// Test when port mapping exists (should update it and check for overlaps)
	mockReader.portMappingExists = true
	binding.PortMapping = nil // Reset port mapping

	err = validator.ValidateForCreation(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that a port mapping was created
	if binding.PortMapping == nil {
		t.Error("Expected port mapping to be created, got nil")
	}

	// Check that the port mapping has the correct ID
	if binding.PortMapping.ResourceIdentifier.Key() != "test-address-group" {
		t.Errorf("Expected port mapping ID to be 'test-address-group', got '%s'", binding.PortMapping.ResourceIdentifier.Key())
	}

	// Check that the port mapping has both the existing service ports and the new service ports
	if len(binding.PortMapping.AccessPorts) != 2 {
		t.Errorf("Expected port mapping to have 2 services, got %d", len(binding.PortMapping.AccessPorts))
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

	// Test when port mapping doesn't exist (should create a new one)
	err := validator.ValidateForUpdate(context.Background(), oldBinding, newBinding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that a port mapping was created
	if newBinding.PortMapping == nil {
		t.Error("Expected port mapping to be created, got nil")
	}

	// Check that the port mapping has the correct ID
	if newBinding.PortMapping.ResourceIdentifier.Key() != "test-address-group" {
		t.Errorf("Expected port mapping ID to be 'test-address-group', got '%s'", newBinding.PortMapping.ResourceIdentifier.Key())
	}

	// Test when port mapping exists (should update it and check for overlaps)
	mockReader.portMappingExists = true
	newBinding.PortMapping = nil // Reset port mapping

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBinding)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that a port mapping was created
	if newBinding.PortMapping == nil {
		t.Error("Expected port mapping to be created, got nil")
	}

	// Test when service reference is changed (should return error)
	newBindingWithChangedService := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns")),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("other-service"),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-address-group"),
		},
	}

	err = validator.ValidateForUpdate(context.Background(), oldBinding, newBindingWithChangedService)
	if err == nil {
		t.Error("Expected error when changing service reference, got nil")
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
	bindingExists      bool
	bindingID          string
	serviceExists      bool
	serviceID          string
	serviceNamespace   string
	addressGroupExists bool
	addressGroupID     string
	portMappingExists  bool
	portMappingID      string
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
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
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

		// Add some existing service ports
		existingServiceRef := models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier("existing-service"),
		}

		existingServicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		// Add TCP port 8080
		existingServicePorts.Ports[models.TCP] = append(
			existingServicePorts.Ports[models.TCP],
			models.PortRange{Start: 8080, End: 8080},
		)

		// Add UDP port range 1000-2000
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
	return nil, nil
}

func (m *MockReaderForAddressGroupBindingValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}
