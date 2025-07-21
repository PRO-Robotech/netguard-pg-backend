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
