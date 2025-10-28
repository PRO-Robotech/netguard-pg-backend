package conditions

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessHostBindingConditions(ctx context.Context, binding *models.HostBinding, syncResult error) error {
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("ConditionManager: Failed to get reader for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// syncResult is ignored for HostBinding as it doesn't sync with external systems
	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "HostBinding committed to backend successfully")

	// Проверяем Host
	hostID := models.ResourceIdentifier{Name: binding.HostRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetHostByID(ctx, hostID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: Host %s not found for %s/%s", hostID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Host %s not found", hostID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Host not found")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "Host validation failed")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check Host %s for %s/%s: %v", hostID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Host %s: %v", hostID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Host validation failed")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "Host validation failed")
		return nil
	}

	// Проверяем AddressGroup
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetAddressGroupByID(ctx, addressGroupID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: AddressGroup %s not found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", addressGroupID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced AddressGroup not found")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "AddressGroup validation failed")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", addressGroupID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", addressGroupID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "AddressGroup validation failed")
		return nil
	}

	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "HostBinding passed validation")
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "HostBinding is ready for use")

	if err := cm.SaveHostBindingConditions(ctx, binding); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for host binding %s/%s: %v", binding.Namespace, binding.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveHostBindingConditions(ctx context.Context, binding *models.HostBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving host binding conditions: %w", err)
	}

	scope := ports.EmptyScope{}

	if err := writer.SyncHostBindings(ctx, []models.HostBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync host binding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit host binding conditions: %w", err)
	}

	return nil
}
