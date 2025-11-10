package conditions

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
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

	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "NetworkBinding committed to backend successfully")
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	if err := bindingValidator.ValidateForPostCommit(ctx, *binding); err != nil {
		klog.Errorf("ConditionManager: NetworkBinding validation failed for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("NetworkBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "NetworkBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "NetworkBinding passed validation")
	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetNetworkByID(ctx, networkID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: Network %s not found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Network %s not found", networkID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Network not found")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check Network %s for %s/%s: %v", networkID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Network %s: %v", networkID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network validation failed")
		return nil
	} else {
		klog.Infof("ConditionManager: Network %s found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
	}

	// Проверяем AddressGroup
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetAddressGroupByID(ctx, addressGroupID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: AddressGroup %s not found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", addressGroupID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", addressGroupID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", addressGroupID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	} else {
		klog.Infof("ConditionManager: AddressGroup %s found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
	}

	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "NetworkBinding is ready for use")

	return nil
}

func (cm *ConditionManager) SaveNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for NetworkBinding conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync NetworkBinding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit NetworkBinding conditions: %w", err)
	}

	return nil
}
