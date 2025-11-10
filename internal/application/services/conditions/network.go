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

func (cm *ConditionManager) ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error {
	network.Meta.ClearErrorCondition()
	network.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("ConditionManager: Failed to get reader for %s/%s: %v", network.Namespace, network.Name, err)
		network.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	if syncResult != nil {
		klog.Errorf("ConditionManager: sgroups sync failed for %s/%s: %v", network.Namespace, network.Name, syncResult)
		network.Meta.SetSyncedCondition(metav1.ConditionFalse, models.ReasonSyncFailed, fmt.Sprintf("Failed to sync with sgroups: %v", syncResult))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network sync with external source failed")
		network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidating, "Validation skipped due to sync failure")
		return nil
	}

	existingReady := network.Meta.GetCondition("Ready")
	isPendingSync := existingReady != nil &&
		existingReady.Status == metav1.ConditionFalse &&
		existingReady.Reason == "PendingSGROUPSync"

	if isPendingSync {
		klog.V(4).InfoS("ConditionManager: Network is pending SGROUP sync, NOT setting Ready=True",
			"namespace", network.Namespace, "name", network.Name)

		if cm.hasPendingOutboxEntry(ctx, "Network", network.Namespace, network.Name) {
			klog.InfoS("ConditionManager: Skipping batch update - OutboxWorker is processing",
				"namespace", network.Namespace,
				"name", network.Name)
			return nil
		}

		validator := validation.NewDependencyValidator(reader)
		networkValidator := validator.GetNetworkValidator()

		if err := networkValidator.ValidateCIDR(network.CIDR); err != nil {
			klog.Errorf("ConditionManager: Network CIDR validation failed for %s/%s: %v", network.Namespace, network.Name, err)
			network.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Network CIDR validation failed: %v", err))
			network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network has validation errors")
			network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("CIDR validation failed: %v", err))
			return nil
		}

		network.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Network passed validation")
		cm.batchConditionUpdate("Network", network)
		return nil
	}

	// OLD BEHAVIOR: For existing resources, set Ready=True
	network.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Network committed to backend and synced with sgroups successfully")

	validator := validation.NewDependencyValidator(reader)
	networkValidator := validator.GetNetworkValidator()

	if err := networkValidator.ValidateCIDR(network.CIDR); err != nil {
		klog.Errorf("ConditionManager: Network CIDR validation failed for %s/%s: %v", network.Namespace, network.Name, err)
		network.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Network CIDR validation failed: %v", err))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network has validation errors")
		network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("CIDR validation failed: %v", err))
		return nil
	}

	network.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Network passed validation")
	network.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Network is ready for use")

	if err := cm.SaveNetworkConditions(ctx, network); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for network %s/%s: %v", network.Namespace, network.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveNetworkConditions(ctx context.Context, network *models.Network) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving network conditions: %w", err)
	}

	scope := ports.EmptyScope{}

	if err := writer.SyncNetworks(ctx, []models.Network{*network}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync network with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit network conditions: %w", err)
	}

	return nil
}
