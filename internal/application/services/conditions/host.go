package conditions

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessHostConditions(ctx context.Context, host *models.Host, syncResult error) error {
	host.Meta.ClearErrorCondition()
	host.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("ConditionManager: Failed to get reader for %s/%s: %v", host.Namespace, host.Name, err)
		host.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		host.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	if syncResult != nil {
		klog.Errorf("ConditionManager: sgroups sync failed for %s/%s: %v", host.Namespace, host.Name, syncResult)
		host.Meta.SetSyncedCondition(metav1.ConditionFalse, models.ReasonSyncFailed, fmt.Sprintf("Failed to sync with sgroups: %v", syncResult))
		host.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Host sync with external source failed")
		host.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidating, "Validation skipped due to sync failure")
		return nil
	}

	existingReady := host.Meta.GetCondition("Ready")
	isPendingSync := existingReady != nil &&
		existingReady.Status == metav1.ConditionFalse &&
		existingReady.Reason == "PendingSGROUPSync"

	if isPendingSync {
		klog.V(4).InfoS("ConditionManager: Host is pending SGROUP sync, NOT setting Ready=True",
			"namespace", host.Namespace, "name", host.Name)

		// Only set Validated condition, leave Ready=False
		host.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Host passed validation")
		// Keep Ready=False (PendingSGROUPSync) - Worker will update after successful sync

		if err := cm.SaveHostConditions(ctx, host); err != nil {
			klog.Errorf("ConditionManager: Failed to save conditions for host %s/%s: %v", host.Namespace, host.Name, err)
			return nil
		}

		return nil
	}

	// OLD BEHAVIOR: For existing resources, set Ready=True
	host.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Host committed to backend and synced with sgroups successfully")
	host.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Host passed validation")
	host.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Host is ready for use")

	if err := cm.SaveHostConditions(ctx, host); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for host %s/%s: %v", host.Namespace, host.Name, err)
		return nil
	}

	return nil
}

func (cm *ConditionManager) SaveHostConditions(ctx context.Context, host *models.Host) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving host conditions: %w", err)
	}

	scope := ports.EmptyScope{}

	if err := writer.SyncHosts(ctx, []models.Host{*host}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync host with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit host conditions: %w", err)
	}

	return nil
}
