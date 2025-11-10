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

func (cm *ConditionManager) ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	policy.Meta.ClearErrorCondition()
	policy.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	policy.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBindingPolicy committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForPostCommit(ctx, *policy); err != nil {
		policy.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBindingPolicy validation failed: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBindingPolicy has validation errors")
		policy.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	policy.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBindingPolicy passed validation")

	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(policy.AddressGroupRef.Name, models.WithNamespace(policy.AddressGroupRef.Namespace))
	_, err = reader.GetAddressGroupByID(ctx, agID)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", policy.AddressGroupRefKey()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", policy.AddressGroupRefKey(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	// Create ResourceIdentifier from NamespacedObjectReference
	serviceID := models.NewResourceIdentifier(policy.ServiceRef.Name, models.WithNamespace(policy.ServiceRef.Namespace))
	_, err = reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", policy.ServiceRefKey()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", policy.ServiceRefKey(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	policy.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "AddressGroupBindingPolicy is ready and operational")

	if err := cm.SaveAddressGroupBindingPolicyConditions(ctx, policy); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for AddressGroupBindingPolicy %s/%s: %v", policy.Namespace, policy.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupBindingPolicy conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncAddressGroupBindingPolicies(ctx, []models.AddressGroupBindingPolicy{*policy}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupBindingPolicy with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupBindingPolicy conditions: %w", err)
	}

	return nil
}
