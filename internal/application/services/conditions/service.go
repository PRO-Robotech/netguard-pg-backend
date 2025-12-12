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

func (cm *ConditionManager) ProcessServiceConditions(ctx context.Context, service *models.Service) error {
	service.Meta.ClearErrorCondition()
	service.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for service %s/%s: %v", service.Namespace, service.Name, err)
		service.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	service.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Service committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForPostCommit(ctx, *service); err != nil {
		klog.Errorf("Service validation failed for %s/%s: %v", service.Namespace, service.Name, err)
		service.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Service validation failed: %v", err))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service has validation errors")
		service.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	service.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Service passed validation")

	missingAddressGroups := []string{}
	for _, agRef := range service.AddressGroups {
		_, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{Name: agRef.Name, Namespace: agRef.Namespace})
		if err == ports.ErrNotFound {
			missingAddressGroups = append(missingAddressGroups, models.AddressGroupRefKey(agRef))
		} else if err != nil {
			klog.Errorf("Failed to check AddressGroup %s for service %s/%s: %v", models.AddressGroupRefKey(agRef), service.Namespace, service.Name, err)
			service.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", models.AddressGroupRefKey(agRef), err))
			service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
			return nil
		}
	}

	if len(missingAddressGroups) > 0 {
		klog.Errorf("Missing AddressGroups for service %s/%s: %v", service.Namespace, service.Name, missingAddressGroups)
		service.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Missing AddressGroups: %v", missingAddressGroups))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroups not found")
		return nil
	}

	existingReady := service.Meta.GetCondition("Ready")
	isPendingSync := existingReady != nil &&
		existingReady.Status == metav1.ConditionFalse &&
		existingReady.Reason == "PendingSGROUPSync"

	if isPendingSync {
		if cm.hasPendingOutboxEntry(ctx, "Service", service.Namespace, service.Name) {
			klog.InfoS("ConditionManager: Skipping batch update - OutboxWorker is processing",
				"namespace", service.Namespace,
				"name", service.Name)
			return nil
		}
		klog.Infof("ConditionManager: Service %s/%s is pending SGROUP sync, NOT overriding Ready=False",
			service.Namespace, service.Name)
		return nil
	}

	service.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Service is ready for use")
	return nil
}

func (cm *ConditionManager) SaveServiceConditions(ctx context.Context, service *models.Service) error {
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for service %s/%s: %w", service.Namespace, service.Name, err)
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncServices(ctx, []models.Service{*service}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync service conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit service conditions with ReadCommitted transaction: %w", err)
		}

		return nil
	}

	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving service conditions: %w", err)
	}

	scope := ports.EmptyScope{}
	if err := writer.SyncServices(ctx, []models.Service{*service}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync service with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit service conditions: %w", err)
	}

	return nil
}
