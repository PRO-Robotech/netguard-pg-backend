package validation_test

import (
	"context"
	"fmt"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupPortMappingValidator_ValidateExists tests the ValidateExists method of AddressGroupPortMappingValidator
func TestAddressGroupPortMappingValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group port mapping for the test ID
	mockReader := &MockReaderForAddressGroupPortMappingValidator{
		mappingExists: true,
		mappingID:     "test-mapping",
	}

	validator := validation.NewAddressGroupPortMappingValidator(mockReader)
	mappingID := models.NewResourceIdentifier("test-mapping")

	// Test when mapping exists
	err := validator.ValidateExists(context.Background(), mappingID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when mapping does not exist
	mockReader.mappingExists = false
	err = validator.ValidateExists(context.Background(), mappingID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupPortMappingValidator_ValidateReferences tests the ValidateReferences method of AddressGroupPortMappingValidator
func TestAddressGroupPortMappingValidator_ValidateReferences(t *testing.T) {
	// Create a mock reader with valid references
	mockReader := &MockReaderForAddressGroupPortMappingValidator{
		serviceExists: true,
		serviceID:     "test-service",
	}

	validator := validation.NewAddressGroupPortMappingValidator(mockReader)

	// Create a port mapping with a service reference
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	accessPorts[models.ServiceRef{
		ResourceIdentifier: models.NewResourceIdentifier("test-service"),
	}] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	mapping := models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
		},
		AccessPorts: accessPorts,
	}

	// Test when all references are valid
	err := validator.ValidateReferences(context.Background(), mapping)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service reference is invalid
	mockReader.serviceExists = false
	err = validator.ValidateReferences(context.Background(), mapping)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestAddressGroupPortMappingValidator_CheckInternalPortOverlaps tests the CheckInternalPortOverlaps method
func TestAddressGroupPortMappingValidator_CheckInternalPortOverlaps(t *testing.T) {
	tests := []struct {
		name    string
		mapping models.AddressGroupPortMapping
		wantErr bool
	}{
		{
			name: "No overlaps",
			mapping: models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service1"),
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 80, End: 80},
								{Start: 443, End: 443},
							},
						},
					},
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service2"),
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 8080, End: 8080},
								{Start: 8443, End: 8443},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "TCP port overlap",
			mapping: models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service1"),
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 80, End: 90},
							},
						},
					},
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service2"),
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 85, End: 95},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "UDP port overlap",
			mapping: models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service1"),
					}: {
						Ports: models.ProtocolPorts{
							models.UDP: []models.PortRange{
								{Start: 53, End: 53},
							},
						},
					},
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service2"),
					}: {
						Ports: models.ProtocolPorts{
							models.UDP: []models.PortRange{
								{Start: 53, End: 53},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Different protocols, no overlap",
			mapping: models.AddressGroupPortMapping{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("test-mapping"),
				},
				AccessPorts: map[models.ServiceRef]models.ServicePorts{
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service1"),
					}: {
						Ports: models.ProtocolPorts{
							models.TCP: []models.PortRange{
								{Start: 80, End: 80},
							},
						},
					},
					models.ServiceRef{
						ResourceIdentifier: models.NewResourceIdentifier("service2"),
					}: {
						Ports: models.ProtocolPorts{
							models.UDP: []models.PortRange{
								{Start: 80, End: 80},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReader := &MockReaderForAddressGroupPortMappingValidator{}
			validator := validation.NewAddressGroupPortMappingValidator(mockReader)
			err := validator.CheckInternalPortOverlaps(tt.mapping)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckInternalPortOverlaps() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// MockReaderForAddressGroupPortMappingValidator is a specialized mock for testing AddressGroupPortMappingValidator
type MockReaderForAddressGroupPortMappingValidator struct {
	mappingExists bool
	mappingID     string
	serviceExists bool
	serviceID     string
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
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

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	if m.mappingExists {
		// Create a port mapping with a service reference
		accessPorts := make(map[models.ServiceRef]models.ServicePorts)
		accessPorts[models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
		}] = models.ServicePorts{
			Ports: models.ProtocolPorts{
				models.TCP: []models.PortRange{{Start: 80, End: 80}},
			},
		}

		mapping := models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.mappingID),
			},
			AccessPorts: accessPorts,
		}
		return consume(mapping)
	}
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if m.serviceExists && id.Key() == m.serviceID {
		return &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
			},
		}, nil
	}
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	if m.mappingExists && id.Key() == m.mappingID {
		// Create a port mapping with a service reference
		accessPorts := make(map[models.ServiceRef]models.ServicePorts)
		accessPorts[models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(m.serviceID),
		}] = models.ServicePorts{
			Ports: models.ProtocolPorts{
				models.TCP: []models.PortRange{{Start: 80, End: 80}},
			},
		}

		return &models.AddressGroupPortMapping{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.mappingID),
			},
			AccessPorts: accessPorts,
		}, nil
	}
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, fmt.Errorf("rule s2s not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, fmt.Errorf("service alias not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("address group binding policy not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupPortMappingValidator) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	// Since this mock is for AddressGroupPortMappingValidator tests, we don't expect this method to be called
	// But we still return a proper error instead of nil, nil
	return nil, fmt.Errorf("IEAgAgRule not found")
}

func (m *MockReaderForAddressGroupPortMappingValidator) Close() error {
	return nil
}
