package models

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for all netguard resources
const (
	// ConditionReady indicates that the resource is ready for use
	ConditionReady string = "Ready"

	// ConditionSynced indicates that the resource has been synchronized with backend
	ConditionSynced string = "Synced"

	// ConditionValidated indicates that the resource has been validated
	ConditionValidated string = "Validated"

	// ConditionError indicates that there is an error with the resource
	ConditionError string = "Error"
)

// Standard condition reasons
const (
	// Ready reasons
	ReasonReady    string = "Ready"
	ReasonNotReady string = "NotReady"
	ReasonPending  string = "Pending"

	// Sync reasons
	ReasonSynced      string = "Synced"
	ReasonSyncFailed  string = "SyncFailed"
	ReasonSyncPending string = "SyncPending"

	// Validation reasons
	ReasonValidated        string = "Validated"
	ReasonValidationFailed string = "ValidationFailed"
	ReasonValidating       string = "Validating"

	// Error reasons
	ReasonError              string = "Error"
	ReasonBackendError       string = "BackendError"
	ReasonConfigurationError string = "ConfigurationError"
	ReasonDependencyError    string = "DependencyError"
	ReasonCleanupError       string = "CleanupError"
)

// NewReadyCondition creates a new Ready condition
func NewReadyCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               ConditionReady,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// NewSyncedCondition creates a new Synced condition
func NewSyncedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               ConditionSynced,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// NewValidatedCondition creates a new Validated condition
func NewValidatedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               ConditionValidated,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// NewErrorCondition creates a new Error condition
func NewErrorCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               ConditionError,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// SetReadyCondition sets Ready condition on Meta
func (m *Meta) SetReadyCondition(status metav1.ConditionStatus, reason, message string) {
	condition := NewReadyCondition(status, reason, message)
	m.SetCondition(condition)
}

// SetSyncedCondition sets Synced condition on Meta
func (m *Meta) SetSyncedCondition(status metav1.ConditionStatus, reason, message string) {
	condition := NewSyncedCondition(status, reason, message)
	m.SetCondition(condition)
}

// SetValidatedCondition sets Validated condition on Meta
func (m *Meta) SetValidatedCondition(status metav1.ConditionStatus, reason, message string) {
	condition := NewValidatedCondition(status, reason, message)
	m.SetCondition(condition)
}

// SetErrorCondition sets Error condition on Meta
func (m *Meta) SetErrorCondition(reason, message string) {
	condition := NewErrorCondition(reason, message)
	m.SetCondition(condition)
}

// ClearErrorCondition removes Error condition from Meta
func (m *Meta) ClearErrorCondition() {
	if m == nil || m.Conditions == nil {
		return
	}

	for i, condition := range m.Conditions {
		if condition.Type == ConditionError {
			// Удаляем условие ошибки
			m.Conditions = append(m.Conditions[:i], m.Conditions[i+1:]...)
			break
		}
	}
}

// IsReady checks if resource is ready
func (m *Meta) IsReady() bool {
	return m.IsConditionTrue(ConditionReady)
}

// IsSynced checks if resource is synced
func (m *Meta) IsSynced() bool {
	return m.IsConditionTrue(ConditionSynced)
}

// IsValidated checks if resource is validated
func (m *Meta) IsValidated() bool {
	return m.IsConditionTrue(ConditionValidated)
}

// HasError checks if resource has error
func (m *Meta) HasError() bool {
	return m.IsConditionTrue(ConditionError)
}
