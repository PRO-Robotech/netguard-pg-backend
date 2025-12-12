package conditions

import (
	"netguard-pg-backend/internal/domain/models"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetDefaultConditions sets default conditions for newly created resources
func (cm *ConditionManager) SetDefaultConditions(resource interface{}) {
	switch r := resource.(type) {
	case *models.Service:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Service is being processed")

	case *models.AddressGroup:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroup is being processed")

	case *models.AddressGroupBinding:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupBinding is being processed")

	case *models.AddressGroupPortMapping:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupPortMapping is being processed")

	case *models.AddressGroupBindingPolicy:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupBindingPolicy is being processed")

	case *models.Network:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Network is being processed")

	case *models.NetworkBinding:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "NetworkBinding is being processed")

	case *models.Host:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Host is being processed")

	case *models.HostBinding:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "HostBinding is being processed")

	case *models.SvcSvcRule:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "SvcSvcRule is being processed")
	}
}
