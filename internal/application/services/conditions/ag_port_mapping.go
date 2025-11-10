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

func (cm *ConditionManager) ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	mapping.Meta.ClearErrorCondition()
	mapping.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	mapping.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupPortMapping committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForPostCommit(ctx, *mapping); err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupPortMapping validation failed: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupPortMapping has validation errors")
		mapping.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	mapping.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupPortMapping passed validation")

	if len(mapping.AccessPorts) == 0 {
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "No access ports configured")
		return nil
	}

	missingServices := []string{}
	for serviceRef := range mapping.AccessPorts {
		_, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{Name: serviceRef.Name, Namespace: serviceRef.Namespace})
		if err == ports.ErrNotFound {
			missingServices = append(missingServices, models.ServiceRefKey(serviceRef))
		} else if err != nil {
			mapping.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Service %s: %v", models.ServiceRefKey(serviceRef), err))
			mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
			return nil
		}
	}

	if len(missingServices) > 0 {
		mapping.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Missing Services: %v", missingServices))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Services not found")
		return nil
	}

	accessPortsCount := len(mapping.AccessPorts)
	mapping.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupPortMapping is ready, %d access ports configured", accessPortsCount))

	if err := cm.SaveAddressGroupPortMappingConditions(ctx, mapping); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for AddressGroupPortMapping %s/%s: %v", mapping.Namespace, mapping.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupPortMapping conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{*mapping}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupPortMapping with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupPortMapping conditions: %w", err)
	}

	return nil
}
