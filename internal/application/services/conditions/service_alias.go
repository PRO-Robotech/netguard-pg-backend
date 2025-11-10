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

func (cm *ConditionManager) ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	alias.Meta.ClearErrorCondition()
	alias.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		alias.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	alias.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "ServiceAlias committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	if err := aliasValidator.ValidateForPostCommit(ctx, *alias); err != nil {
		alias.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("ServiceAlias validation failed: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceAlias has validation errors")
		alias.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	alias.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "ServiceAlias passed validation")

	// Create ResourceIdentifier from ObjectReference - service is in same namespace as alias
	serviceID := models.NewResourceIdentifier(alias.ServiceRef.Name, models.WithNamespace(alias.Namespace))
	_, err = reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		alias.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Referenced Service %s not found", alias.ServiceRefKey()))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Service not found")
		return nil
	} else if err != nil {
		alias.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get referenced Service %s: %v", alias.ServiceRefKey(), err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	alias.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "ServiceAlias is ready and operational")

	if err := cm.SaveServiceAliasConditions(ctx, alias); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for service alias %s/%s: %v", alias.Namespace, alias.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for service alias conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncServiceAliases(ctx, []models.ServiceAlias{*alias}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync service alias with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit service alias conditions: %w", err)
	}

	return nil
}
