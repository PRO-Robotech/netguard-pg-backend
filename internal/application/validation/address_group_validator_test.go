package validation_test

import (
	"context"
	"fmt"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupValidator_ValidateExists tests the ValidateExists method of AddressGroupValidator
func TestAddressGroupValidator_ValidateExists(t *testing.T) {
	// Create a custom mock reader that returns an address group for the test ID
	mockReader := &MockReaderForAddressGroupValidator{
		addressGroupExists:    true,
		addressGroupName:      "test-address-group",
		addressGroupNamespace: "default",
	}

	validator := validation.NewAddressGroupValidator(mockReader)
	addressGroupID := models.NewResourceIdentifier("test-address-group", models.WithNamespace("default"))

	// Test when address group exists
	err := validator.ValidateExists(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when address group does not exist
	mockReader.addressGroupExists = false
	err = validator.ValidateExists(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestAddressGroupValidator_CheckDependencies tests the CheckDependencies method of AddressGroupValidator
func TestAddressGroupValidator_CheckDependencies(t *testing.T) {
	// Create a mock reader with no dependencies
	mockReader := &MockReaderForAddressGroupValidator{
		addressGroupName:      "test-address-group",
		addressGroupNamespace: "default",
		hasServiceRefs:        false,
		hasBindingRefs:        false,
	}

	validator := validation.NewAddressGroupValidator(mockReader)
	addressGroupID := models.NewResourceIdentifier("test-address-group", models.WithNamespace("default"))

	// Test when no dependencies exist
	err := validator.CheckDependencies(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test when service dependency exists
	mockReader.hasServiceRefs = true
	err = validator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for service dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Test when address group binding dependency exists
	mockReader.hasServiceRefs = false
	mockReader.hasBindingRefs = true
	err = validator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for address group binding dependency, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}
}

// MockReaderForAddressGroupValidator is a specialized mock for testing AddressGroupValidator
type MockReaderForAddressGroupValidator struct {
	addressGroupExists    bool
	addressGroupName      string
	addressGroupNamespace string
	hasServiceRefs        bool
	hasBindingRefs        bool
}

func (m *MockReaderForAddressGroupValidator) Close() error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	if m.hasServiceRefs {
		service := models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-service", models.WithNamespace(m.addressGroupNamespace)),
			},
			AddressGroups: []models.AddressGroupRef{
				models.NewAddressGroupRef(m.addressGroupName, models.WithNamespace(m.addressGroupNamespace)),
			},
		}
		return consume(service)
	}
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	if m.addressGroupExists {
		addressGroup := models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(m.addressGroupName, models.WithNamespace(m.addressGroupNamespace)),
			},
		}
		return consume(addressGroup)
	}
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	if m.hasBindingRefs {
		binding := models.AddressGroupBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace(m.addressGroupNamespace)),
			},
			AddressGroupRef: models.NewAddressGroupRef(m.addressGroupName, models.WithNamespace(m.addressGroupNamespace)),
		}
		return consume(binding)
	}
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupValidator) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	return nil, fmt.Errorf("service not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	expectedID := models.NewResourceIdentifier(m.addressGroupName, models.WithNamespace(m.addressGroupNamespace))
	if m.addressGroupExists && id.Key() == expectedID.Key() {
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: expectedID,
			},
		}, nil
	}
	return nil, fmt.Errorf("address group not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, fmt.Errorf("address group binding not found")
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("address group port mapping not found")
}

func (m *MockReaderForAddressGroupValidator) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("address group binding policy not found")
}

func (m *MockReaderForAddressGroupValidator) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForAddressGroupValidator) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nil, fmt.Errorf("network binding not found")
}

func (m *MockReaderForAddressGroupValidator) ListHosts(ctx context.Context, consume func(models.Host) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	return nil, fmt.Errorf("host not found")
}

func (m *MockReaderForAddressGroupValidator) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	return nil, fmt.Errorf("host binding not found")
}

func (m *MockReaderForAddressGroupValidator) GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error) {
	return nil, fmt.Errorf("network not found")
}

func (m *MockReaderForAddressGroupValidator) GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error) {
	return nil, nil
}

func (m *MockReaderForAddressGroupValidator) ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) ListSvcFqdnRules(ctx context.Context, consume func(models.SvcFqdnRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	return nil, fmt.Errorf("svc svc rule not found")
}

func (m *MockReaderForAddressGroupValidator) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	return nil, fmt.Errorf("svc fqdn rule not found")
}

func (m *MockReaderForAddressGroupValidator) ListIECidrSvcRules(ctx context.Context, consume func(models.IECidrSvcRule) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForAddressGroupValidator) GetIECidrSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IECidrSvcRule, error) {
	return nil, fmt.Errorf("ie cidr svc rule not found")
}
