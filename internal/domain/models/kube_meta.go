package models

import (
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Meta stores Kubernetes-specific metadata that must survive round-trip
// through the aggregated API server and backend storage.
// All fields are optional and may be empty when the object is first created
// by a client; the API server or backend will fill them where appropriate.
type Meta struct {
	UID             string            `json:"uid,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Generation      int64             `json:"generation,omitempty"`
	CreationTS      metav1.Time       `json:"creationTimestamp,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`

	// GeneratedName is an optional prefix, used by the server, to generate a unique name
	// ONLY IF the Name field has not been provided. Stored for round-trip compatibility.
	GeneratedName string `json:"generatedName,omitempty"`

	// ManagedFields stores Server-Side Apply field ownership information
	// This must be preserved across backend operations to support field management
	ManagedFields []metav1.ManagedFieldsEntry `json:"managedFields,omitempty"`

	// Finalizers is a list of finalizers that must be processed before the object can be deleted
	Finalizers []string `json:"finalizers,omitempty"`

	// Status management - формируется Backend, отображается в Status клиентам
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// TouchOnCreate initializes meta fields that are set exactly once during
// object creation.
func (m *Meta) TouchOnCreate() {
	if m == nil {
		return
	}
	if m.CreationTS.IsZero() {
		m.CreationTS = metav1.Now()
	}
	if m.Generation == 0 {
		m.Generation = 1
	}
	if m.UID == "" {
		m.UID = uuid.NewString()
	}
	// Инициализируем ObservedGeneration текущим Generation
	if m.ObservedGeneration == 0 {
		m.ObservedGeneration = m.Generation
	}
}

// TouchOnWrite updates fields that must change on every write operation.
// uid remains immutable once set.
func (m *Meta) TouchOnWrite(newRV string) {
	if m == nil {
		return
	}
	m.ResourceVersion = newRV
	// Обновляем ObservedGeneration при изменении
	m.ObservedGeneration = m.Generation
}

// SetCondition добавляет или обновляет условие в списке
func (m *Meta) SetCondition(condition metav1.Condition) {
	if m == nil {
		return
	}

	if m.Conditions == nil {
		m.Conditions = []metav1.Condition{}
	}

	// Поиск существующего условия
	for i, existing := range m.Conditions {
		if existing.Type == condition.Type {
			// Обновляем существующее условие
			m.Conditions[i] = condition
			return
		}
	}

	// Добавляем новое условие
	m.Conditions = append(m.Conditions, condition)
}

// GetCondition возвращает условие по типу
func (m *Meta) GetCondition(conditionType string) *metav1.Condition {
	if m == nil || m.Conditions == nil {
		return nil
	}

	for _, condition := range m.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// IsConditionTrue проверяет, является ли условие истинным
func (m *Meta) IsConditionTrue(conditionType string) bool {
	condition := m.GetCondition(conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// GetConditions returns the conditions slice
func (m *Meta) GetConditions() []metav1.Condition {
	if m == nil {
		return nil
	}
	return m.Conditions
}

// SetConditions sets the conditions slice
func (m *Meta) SetConditions(conditions []metav1.Condition) {
	if m == nil {
		return
	}
	m.Conditions = conditions
}

// GetManagedFields returns the managedFields slice
func (m *Meta) GetManagedFields() []metav1.ManagedFieldsEntry {
	if m == nil {
		return nil
	}
	return m.ManagedFields
}

// SetManagedFields sets the managedFields slice
func (m *Meta) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	if m == nil {
		return
	}
	m.ManagedFields = managedFields
}

// AddManagedField adds or updates a managed field entry
func (m *Meta) AddManagedField(entry metav1.ManagedFieldsEntry) {
	if m == nil {
		return
	}

	if m.ManagedFields == nil {
		m.ManagedFields = []metav1.ManagedFieldsEntry{}
	}

	// Find existing entry for the same manager and operation
	for i, existing := range m.ManagedFields {
		if existing.Manager == entry.Manager &&
			existing.Operation == entry.Operation &&
			existing.Subresource == entry.Subresource {
			// Update existing entry
			m.ManagedFields[i] = entry
			return
		}
	}

	// Add new entry
	m.ManagedFields = append(m.ManagedFields, entry)
}

// RemoveManagedFieldsByManager removes all managed field entries for a specific manager
func (m *Meta) RemoveManagedFieldsByManager(manager string) {
	if m == nil || m.ManagedFields == nil {
		return
	}

	filtered := make([]metav1.ManagedFieldsEntry, 0, len(m.ManagedFields))
	for _, entry := range m.ManagedFields {
		if entry.Manager != manager {
			filtered = append(filtered, entry)
		}
	}
	m.ManagedFields = filtered
}
