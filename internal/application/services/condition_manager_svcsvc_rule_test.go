package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/patterns"
)

// MockReader is a minimal mock implementation for testing
type MockSvcSvcRuleReader struct {
	mock.Mock
}

func (m *MockSvcSvcRuleReader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Service), args.Error(1)
}

func (m *MockSvcSvcRuleReader) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Stub implementations for other required methods
func (m *MockSvcSvcRuleReader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListHosts(ctx context.Context, consume func(models.Host) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope ports.Scope) error {
	return nil
}
func (m *MockSvcSvcRuleReader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	return nil, nil
}
func (m *MockSvcSvcRuleReader) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	return nil, nil
}

// MockRegistry is a minimal mock implementation for testing
type MockSvcSvcRuleRegistry struct {
	mock.Mock
}

func (m *MockSvcSvcRuleRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ports.Reader), args.Error(1)
}

func (m *MockSvcSvcRuleRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	return nil, nil
}

func (m *MockSvcSvcRuleRegistry) ReaderFromWriter(ctx context.Context, writer ports.Writer) (ports.Reader, error) {
	return nil, nil
}

func (m *MockSvcSvcRuleRegistry) ReaderWithReadCommitted(ctx context.Context) (ports.Reader, error) {
	return nil, nil
}

func (m *MockSvcSvcRuleRegistry) Close() error {
	return nil
}

func (m *MockSvcSvcRuleRegistry) Subject() patterns.Subject {
	return nil
}

// TestProcessSvcSvcRuleConditions_BothServicesExist tests the happy path where both ServiceFrom and ServiceTo exist
func TestProcessSvcSvcRuleConditions_BothServicesExist(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReader := new(MockSvcSvcRuleReader)
	mockRegistry := new(MockSvcSvcRuleRegistry)

	// Setup mock responses
	mockRegistry.On("Reader", ctx).Return(mockReader, nil)
	mockReader.On("Close").Return(nil)

	serviceFrom := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "service-from",
			},
		},
	}

	serviceTo := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "service-to",
			},
		},
	}

	mockReader.On("GetServiceByID", ctx, models.ResourceIdentifier{
		Name:      "service-from",
		Namespace: "test-ns",
	}).Return(serviceFrom, nil)

	mockReader.On("GetServiceByID", ctx, models.ResourceIdentifier{
		Name:      "service-to",
		Namespace: "test-ns",
	}).Return(serviceTo, nil)

	cm := NewConditionManager(mockRegistry)

	rule := &models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "test-rule",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-from",
			},
			Namespace: "test-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-to",
			},
			Namespace: "test-ns",
		},
		Action:   models.ActionAccept,
		Priority: 100,
		Meta:     models.Meta{},
	}

	// Act
	err := cm.ProcessSvcSvcRuleConditions(ctx, rule)

	// Assert
	assert.NoError(t, err)
	assert.True(t, rule.Meta.IsReady(), "SvcSvcRule should be ready when both services exist")

	// Check Validated condition
	validatedCond := rule.Meta.GetCondition("Validated")
	assert.NotNil(t, validatedCond)
	assert.Equal(t, metav1.ConditionTrue, validatedCond.Status)
	assert.Equal(t, "Validated", validatedCond.Reason)

	// Check Ready condition
	readyCond := rule.Meta.GetCondition("Ready")
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, "Ready", readyCond.Reason)
	assert.Contains(t, readyCond.Message, "all services exist")

	// Check Synced condition
	syncedCond := rule.Meta.GetCondition("Synced")
	assert.NotNil(t, syncedCond)
	assert.Equal(t, metav1.ConditionTrue, syncedCond.Status)

	mockRegistry.AssertExpectations(t)
	mockReader.AssertExpectations(t)
}

// TestProcessSvcSvcRuleConditions_ServiceFromMissing tests when ServiceFrom is missing
func TestProcessSvcSvcRuleConditions_ServiceFromMissing(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReader := new(MockSvcSvcRuleReader)
	mockRegistry := new(MockSvcSvcRuleRegistry)

	mockRegistry.On("Reader", ctx).Return(mockReader, nil)
	mockReader.On("Close").Return(nil)

	// ServiceFrom does NOT exist
	mockReader.On("GetServiceByID", ctx, models.ResourceIdentifier{
		Name:      "service-from",
		Namespace: "test-ns",
	}).Return(nil, ports.ErrNotFound)

	cm := NewConditionManager(mockRegistry)

	rule := &models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "test-rule",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-from",
			},
			Namespace: "test-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-to",
			},
			Namespace: "test-ns",
		},
		Action:   models.ActionAccept,
		Priority: 100,
		Meta:     models.Meta{},
	}

	// Act
	err := cm.ProcessSvcSvcRuleConditions(ctx, rule)

	// Assert
	assert.NoError(t, err) // ProcessConditions should not return error, only set conditions
	assert.False(t, rule.Meta.IsReady(), "SvcSvcRule should NOT be ready when ServiceFrom is missing")

	// Check Ready condition
	readyCond := rule.Meta.GetCondition("Ready")
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, "NotReady", readyCond.Reason)
	assert.Contains(t, readyCond.Message, "ServiceFrom not found")

	// Check Validated condition
	validatedCond := rule.Meta.GetCondition("Validated")
	assert.NotNil(t, validatedCond)
	assert.Equal(t, metav1.ConditionFalse, validatedCond.Status)
	assert.Equal(t, "ValidationFailed", validatedCond.Reason)
	assert.Contains(t, validatedCond.Message, "ServiceFrom dependency missing")

	// Check Error condition
	errorCond := rule.Meta.GetCondition("Error")
	assert.NotNil(t, errorCond)
	assert.Contains(t, errorCond.Message, "ServiceFrom")
	assert.Contains(t, errorCond.Message, "not found")

	mockRegistry.AssertExpectations(t)
	mockReader.AssertExpectations(t)
}

// TestProcessSvcSvcRuleConditions_ServiceToMissing tests when ServiceTo is missing
func TestProcessSvcSvcRuleConditions_ServiceToMissing(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReader := new(MockSvcSvcRuleReader)
	mockRegistry := new(MockSvcSvcRuleRegistry)

	mockRegistry.On("Reader", ctx).Return(mockReader, nil)
	mockReader.On("Close").Return(nil)

	serviceFrom := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "service-from",
			},
		},
	}

	// ServiceFrom exists
	mockReader.On("GetServiceByID", ctx, models.ResourceIdentifier{
		Name:      "service-from",
		Namespace: "test-ns",
	}).Return(serviceFrom, nil)

	// ServiceTo does NOT exist
	mockReader.On("GetServiceByID", ctx, models.ResourceIdentifier{
		Name:      "service-to",
		Namespace: "test-ns",
	}).Return(nil, ports.ErrNotFound)

	cm := NewConditionManager(mockRegistry)

	rule := &models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "test-rule",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-from",
			},
			Namespace: "test-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-to",
			},
			Namespace: "test-ns",
		},
		Action:   models.ActionAccept,
		Priority: 100,
		Meta:     models.Meta{},
	}

	// Act
	err := cm.ProcessSvcSvcRuleConditions(ctx, rule)

	// Assert
	assert.NoError(t, err)
	assert.False(t, rule.Meta.IsReady(), "SvcSvcRule should NOT be ready when ServiceTo is missing")

	// Check Ready condition
	readyCond := rule.Meta.GetCondition("Ready")
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, "NotReady", readyCond.Reason)
	assert.Contains(t, readyCond.Message, "ServiceTo not found")

	// Check Validated condition
	validatedCond := rule.Meta.GetCondition("Validated")
	assert.NotNil(t, validatedCond)
	assert.Equal(t, metav1.ConditionFalse, validatedCond.Status)
	assert.Equal(t, "ValidationFailed", validatedCond.Reason)
	assert.Contains(t, validatedCond.Message, "ServiceTo dependency missing")

	// Check Error condition
	errorCond := rule.Meta.GetCondition("Error")
	assert.NotNil(t, errorCond)
	assert.Contains(t, errorCond.Message, "ServiceTo")
	assert.Contains(t, errorCond.Message, "not found")

	mockRegistry.AssertExpectations(t)
	mockReader.AssertExpectations(t)
}

// TestProcessSvcSvcRuleConditions_ReaderError tests when Reader fails
func TestProcessSvcSvcRuleConditions_ReaderError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRegistry := new(MockSvcSvcRuleRegistry)

	// Reader fails to be created
	mockRegistry.On("Reader", ctx).Return(nil, assert.AnError)

	cm := NewConditionManager(mockRegistry)

	rule := &models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-ns",
				Name:      "test-rule",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-from",
			},
			Namespace: "test-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "service-to",
			},
			Namespace: "test-ns",
		},
		Action:   models.ActionAccept,
		Priority: 100,
		Meta:     models.Meta{},
	}

	// Act
	err := cm.ProcessSvcSvcRuleConditions(ctx, rule)

	// Assert
	assert.NoError(t, err) // ProcessConditions should not return error on backend errors
	assert.False(t, rule.Meta.IsReady(), "SvcSvcRule should NOT be ready when Reader fails")

	// Check Ready condition
	readyCond := rule.Meta.GetCondition("Ready")
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, "NotReady", readyCond.Reason)
	assert.Contains(t, readyCond.Message, "Backend validation unavailable")

	// Check Error condition
	errorCond := rule.Meta.GetCondition("Error")
	assert.NotNil(t, errorCond)
	assert.Equal(t, "BackendError", errorCond.Reason)
	assert.Contains(t, errorCond.Message, "Failed to get reader")

	mockRegistry.AssertExpectations(t)
}
