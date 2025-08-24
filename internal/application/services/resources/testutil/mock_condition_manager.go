package testutil

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"netguard-pg-backend/internal/domain/models"
)

// MockConditionManager implements a test-friendly condition manager
type MockConditionManager struct {
	mu         sync.RWMutex
	conditions map[string][]metav1.Condition // resource key -> conditions
}

// NewMockConditionManager creates a new mock condition manager
func NewMockConditionManager() *MockConditionManager {
	return &MockConditionManager{
		conditions: make(map[string][]metav1.Condition),
	}
}

// SetCondition sets a condition for a resource
func (m *MockConditionManager) SetCondition(ctx context.Context, resourceID models.ResourceIdentifier, condition metav1.Condition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := resourceID.Key()
	if m.conditions[key] == nil {
		m.conditions[key] = make([]metav1.Condition, 0)
	}

	// Update existing condition or add new one
	found := false
	for i, existingCondition := range m.conditions[key] {
		if existingCondition.Type == condition.Type {
			m.conditions[key][i] = condition
			found = true
			break
		}
	}

	if !found {
		m.conditions[key] = append(m.conditions[key], condition)
	}

	return nil
}

// GetConditions returns all conditions for a resource
func (m *MockConditionManager) GetConditions(ctx context.Context, resourceID models.ResourceIdentifier) ([]metav1.Condition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := resourceID.Key()
	conditions := m.conditions[key]
	if conditions == nil {
		return []metav1.Condition{}, nil
	}

	// Return a copy to prevent race conditions
	result := make([]metav1.Condition, len(conditions))
	copy(result, conditions)
	return result, nil
}

// ClearConditions removes all conditions for a resource
func (m *MockConditionManager) ClearConditions(ctx context.Context, resourceID models.ResourceIdentifier) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := resourceID.Key()
	delete(m.conditions, key)
	return nil
}

// GetAllConditions returns all conditions for all resources (useful for testing)
func (m *MockConditionManager) GetAllConditions() map[string][]metav1.Condition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]metav1.Condition)
	for key, conditions := range m.conditions {
		result[key] = make([]metav1.Condition, len(conditions))
		copy(result[key], conditions)
	}
	return result
}

// Reset clears all conditions (useful for test cleanup)
func (m *MockConditionManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.conditions = make(map[string][]metav1.Condition)
}

// ProcessRuleS2SConditions processes conditions for RuleS2S resources
func (m *MockConditionManager) ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	// Mock implementation - just set a Ready condition
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "RuleS2S conditions processed by mock manager",
	}

	return m.SetCondition(ctx, rule.SelfRef.ResourceIdentifier, condition)
}

// ProcessIEAgAgRuleConditions processes conditions for IEAgAgRule resources
func (m *MockConditionManager) ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	// Mock implementation - just set a Ready condition
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "IEAgAgRule conditions processed by mock manager",
	}

	return m.SetCondition(ctx, rule.SelfRef.ResourceIdentifier, condition)
}

// SaveResourceConditions saves conditions for a resource
func (m *MockConditionManager) SaveResourceConditions(ctx context.Context, resource interface{}) error {
	// Mock implementation - already saved in ProcessRuleS2SConditions
	return nil
}

// AddressGroupConditionManagerInterface implementation
// ProcessAddressGroupConditions processes conditions for AddressGroup resources
func (m *MockConditionManager) ProcessAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "AddressGroup conditions processed by mock manager",
	}
	return m.SetCondition(ctx, addressGroup.SelfRef.ResourceIdentifier, condition)
}

// ProcessAddressGroupBindingConditions processes conditions for AddressGroupBinding resources
func (m *MockConditionManager) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "AddressGroupBinding conditions processed by mock manager",
	}
	return m.SetCondition(ctx, binding.SelfRef.ResourceIdentifier, condition)
}

// ProcessAddressGroupPortMappingConditions processes conditions for AddressGroupPortMapping resources
func (m *MockConditionManager) ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "AddressGroupPortMapping conditions processed by mock manager",
	}
	return m.SetCondition(ctx, mapping.SelfRef.ResourceIdentifier, condition)
}

// ProcessAddressGroupBindingPolicyConditions processes conditions for AddressGroupBindingPolicy resources
func (m *MockConditionManager) ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "AddressGroupBindingPolicy conditions processed by mock manager",
	}
	return m.SetCondition(ctx, policy.SelfRef.ResourceIdentifier, condition)
}

// Save methods for condition persistence
func (m *MockConditionManager) SaveAddressGroupConditions(ctx context.Context, addressGroup *models.AddressGroup) error {
	// Mock implementation - conditions already saved in Process methods
	return nil
}

func (m *MockConditionManager) SaveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	// Mock implementation - conditions already saved in Process methods
	return nil
}

func (m *MockConditionManager) SaveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	// Mock implementation - conditions already saved in Process methods
	return nil
}

func (m *MockConditionManager) SaveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	// Mock implementation - conditions already saved in Process methods
	return nil
}

// NetworkConditionManagerInterface implementation
// ProcessNetworkConditions processes conditions for Network resources
func (m *MockConditionManager) ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error {
	var condition metav1.Condition
	if syncResult != nil {
		condition = metav1.Condition{
			Type:               models.ConditionSynced,
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "MockSyncError",
			Message:            fmt.Sprintf("Network sync failed: %v", syncResult),
		}
	} else {
		condition = metav1.Condition{
			Type:               models.ConditionSynced,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "MockSynced",
			Message:            "Network conditions processed by mock manager",
		}
	}
	return m.SetCondition(ctx, network.SelfRef.ResourceIdentifier, condition)
}

// NetworkBindingConditionManagerInterface implementation
// ProcessNetworkBindingConditions processes conditions for NetworkBinding resources
func (m *MockConditionManager) ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "NetworkBinding conditions processed by mock manager",
	}
	return m.SetCondition(ctx, binding.SelfRef.ResourceIdentifier, condition)
}

// ServiceConditionManagerInterface implementation
// ProcessServiceConditions processes conditions for Service resources
func (m *MockConditionManager) ProcessServiceConditions(ctx context.Context, service *models.Service) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "Service conditions processed by mock manager",
	}
	return m.SetCondition(ctx, service.SelfRef.ResourceIdentifier, condition)
}

// ProcessServiceAliasConditions processes conditions for ServiceAlias resources
func (m *MockConditionManager) ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	condition := metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MockProcessed",
		Message:            "ServiceAlias conditions processed by mock manager",
	}
	return m.SetCondition(ctx, alias.SelfRef.ResourceIdentifier, condition)
}
