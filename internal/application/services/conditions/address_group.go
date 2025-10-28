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
	// LOG 1: Entry point - what conditions came in
	klog.InfoS("[DEBUG] ConditionManager: ProcessAddressGroupConditions ENTRY",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"conditions", formatConditions(ag.Meta.Conditions))

	ag.Meta.ClearErrorCondition()
	ag.Meta.TouchOnWrite("v1")

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

	// LOG 2: Check Ready condition details
	existingReady := ag.Meta.GetCondition("Ready")
	klog.InfoS("[DEBUG] ConditionManager: Checking Ready condition",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"ready_exists", existingReady != nil,
		"ready_status", getConditionStatus(existingReady),
		"ready_reason", getConditionReason(existingReady))

	isPendingSync := existingReady != nil &&
		existingReady.Status == metav1.ConditionFalse &&
		existingReady.Reason == "PendingSGROUPSync"

	// LOG 3: Decision result (KEY LOG!)
	klog.InfoS("ConditionManager: isPendingSync decision",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"isPendingSync", isPendingSync,
		"action", map[bool]string{true: "KEEP_PENDING", false: "SET_READY_TRUE"}[isPendingSync])

	if isPendingSync {
		klog.InfoS("ConditionManager: KEEPING PendingSGROUPSync",
			"namespace", ag.Namespace,
			"name", ag.Name)

		// Only set Validated condition, leave Ready=False
		ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")
		// Keep Ready=False (PendingSGROUPSync) - Worker will update after successful sync
		cm.batchConditionUpdate("AddressGroup", ag)
		return nil
	}

	// LOG 4: OLD BEHAVIOR (this might be the bug!)
	klog.InfoS("ConditionManager: ⚠️ Setting Ready=True (OLD BEHAVIOR - potential bug)",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"reason", "isPendingSync was false")

	// OLD BEHAVIOR: For existing resources, set Ready=True
	ag.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Address group is ready and operational")
	ag.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Address group successfully synced to backend and SGROUP")
	ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")
	cm.batchConditionUpdate("AddressGroup", ag)
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
