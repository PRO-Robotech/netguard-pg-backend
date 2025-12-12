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

func (cm *ConditionManager) ProcessAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
	klog.InfoS("[DEBUG] ConditionManager: ProcessAddressGroupConditions ENTRY",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"conditions", formatConditions(ag.Meta.Conditions))
	ag.Meta.ClearErrorCondition()
	ag.Meta.TouchOnWrite("v1")
	ag.Meta.DeduplicateConditions()
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for AddressGroup %s/%s: %v", ag.Namespace, ag.Name, err)
		ag.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader: %v", err))
		ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend reader unavailable")
		return err
	}
	defer reader.Close()
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()
	if err := addressGroupValidator.ValidateForPostCommit(ctx, *ag); err != nil {
		klog.Errorf("AddressGroup validation failed for %s/%s: %v", ag.Namespace, ag.Name, err)
		ag.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Post-commit validation failed: %v", err))
		ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Address group validation failed")
		return err
	}
	pendingSyncCond := ag.Meta.GetCondition("PendingSync")
	isPendingSync := pendingSyncCond != nil && pendingSyncCond.Status == metav1.ConditionTrue

	existingReady := ag.Meta.GetCondition("Ready")
	readyReasonPending := existingReady != nil &&
		existingReady.Status == metav1.ConditionFalse &&
		(existingReady.Reason == "PendingSGROUPSync" ||
			existingReady.Reason == "BindingDeleting" ||
			existingReady.Reason == "AggregationUpdate")

	if isPendingSync || readyReasonPending {
		klog.InfoS("ConditionManager: resource is pending sync",
			"namespace", ag.Namespace,
			"name", ag.Name,
			"pending_sync_flag", isPendingSync,
			"ready_reason", getConditionReason(existingReady))

		if cm.hasPendingOutboxEntry(ctx, "AddressGroup", ag.Namespace, ag.Name) {
			klog.InfoS("ConditionManager: pending outbox entry detected, keeping Ready=False",
				"namespace", ag.Namespace,
				"name", ag.Name)
			return nil
		}
	}

	// All sync operations completed, set healthy conditions
	ag.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Address group is ready and operational")
	ag.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Address group successfully synced to backend and SGROUP")
	ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")
	ag.Meta.SetCondition(metav1.Condition{
		Type:               "PendingSync",
		Status:             metav1.ConditionFalse,
		Reason:             "SyncComplete",
		Message:            "All synchronization operations completed",
		LastTransitionTime: metav1.Now(),
	})

	return nil
}
func (cm *ConditionManager) SaveAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for AddressGroup %s/%s: %w", ag.Namespace, ag.Name, err)
		}
		scope := ports.EmptyScope{}
		if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{*ag}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync AddressGroup conditions with ReadCommitted transaction: %w", err)
		}
		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit AddressGroup conditions with ReadCommitted transaction: %w", err)
		}
		return nil
	}
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving address group conditions: %w", err)
	}
	scope := ports.EmptyScope{}
	if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{*ag}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync address group with conditions: %w", err)
	}
	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit address group conditions: %w", err)
	}
	return nil
}
