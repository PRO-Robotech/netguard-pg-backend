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

func (cm *ConditionManager) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBinding committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForPostCommit(ctx, binding); err != nil {
		klog.Errorf("AddressGroupBinding validation failed for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBinding passed validation")

	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.Namespace))
	service, err := reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		klog.Errorf("Service %s not found for binding %s/%s", binding.ServiceRefKey(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", binding.ServiceRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to get Service %s for binding %s/%s: %v", binding.ServiceRefKey(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", binding.ServiceRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	_, err = reader.GetAddressGroupByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("AddressGroup %s not found for binding %s/%s", binding.AddressGroupRefKey(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", binding.AddressGroupRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to get AddressGroup %s for binding %s/%s: %v", binding.AddressGroupRefKey(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", binding.AddressGroupRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	if err := validation.CheckPortOverlaps(*service, models.AddressGroupPortMapping{}); err != nil {
		klog.Errorf("Port overlap detected for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Port overlap detected: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port conflicts detected")
		return nil
	}

	portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("AddressGroupPortMapping not created for binding %s/%s", binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, "AddressGroupPortMapping not created")
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping was not created")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to verify port mapping for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to verify port mapping: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping verification failed")
		return nil
	}

	accessPortsCount := len(portMapping.AccessPorts)
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupBinding is ready, %d access ports configured", accessPortsCount))

	if err := cm.SaveAddressGroupBindingConditions(ctx, binding); err != nil {
		klog.Errorf("Failed to save conditions for AddressGroupBinding %s/%s: %v", binding.Namespace, binding.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for address group binding conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync address group binding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit address group binding conditions: %w", err)
	}

	return nil
}
