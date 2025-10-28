package models

import (
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Meta struct {
	UID                string                      `json:"uid,omitempty"`
	ResourceVersion    string                      `json:"resourceVersion,omitempty"`
	Generation         int64                       `json:"generation,omitempty"`
	CreationTS         metav1.Time                 `json:"creationTimestamp,omitempty"`
	DeletionTS         *metav1.Time                `json:"deletionTimestamp,omitempty"`
	Labels             map[string]string           `json:"labels,omitempty"`
	Annotations        map[string]string           `json:"annotations,omitempty"`
	GeneratedName      string                      `json:"generatedName,omitempty"`
	ManagedFields      []metav1.ManagedFieldsEntry `json:"managedFields,omitempty"`
	Finalizers         []string                    `json:"finalizers,omitempty"`
	Conditions         []metav1.Condition          `json:"conditions,omitempty"`
	ObservedGeneration int64                       `json:"observedGeneration,omitempty"`
}

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
	if m.ObservedGeneration == 0 {
		m.ObservedGeneration = m.Generation
	}
}
func (m *Meta) TouchOnWrite(newRV string) {
	if m == nil {
		return
	}
	m.ResourceVersion = newRV
	m.ObservedGeneration = m.Generation
}
func (m *Meta) SetCondition(condition metav1.Condition) {
	if m == nil {
		return
	}
	if m.Conditions == nil {
		m.Conditions = []metav1.Condition{}
	}
	for i, existing := range m.Conditions {
		if existing.Type == condition.Type {
			m.Conditions[i] = condition
			return
		}
	}
	m.Conditions = append(m.Conditions, condition)
}
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
func (m *Meta) IsConditionTrue(conditionType string) bool {
	condition := m.GetCondition(conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}
func (m *Meta) GetConditions() []metav1.Condition {
	if m == nil {
		return nil
	}
	return m.Conditions
}
func (m *Meta) SetConditions(conditions []metav1.Condition) {
	if m == nil {
		return
	}
	m.Conditions = conditions
}
func (m *Meta) GetManagedFields() []metav1.ManagedFieldsEntry {
	if m == nil {
		return nil
	}
	return m.ManagedFields
}
func (m *Meta) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	if m == nil {
		return
	}
	m.ManagedFields = managedFields
}
func (m *Meta) AddManagedField(entry metav1.ManagedFieldsEntry) {
	if m == nil {
		return
	}
	if m.ManagedFields == nil {
		m.ManagedFields = []metav1.ManagedFieldsEntry{}
	}
	for i, existing := range m.ManagedFields {
		if existing.Manager == entry.Manager &&
			existing.Operation == entry.Operation &&
			existing.Subresource == entry.Subresource {
			m.ManagedFields[i] = entry
			return
		}
	}
	m.ManagedFields = append(m.ManagedFields, entry)
}
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
