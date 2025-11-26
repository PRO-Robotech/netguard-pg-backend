package conditions

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessSvcSvcRuleConditions(ctx context.Context, rule *models.SvcSvcRule) error {
	changed := rule.Meta.ClearErrorConditionIfPresent()
	flushUpdate := func() {
		if !changed {
			return
		}
		rule.Meta.TouchOnWrite("v1")
		cm.batchConditionUpdate("SvcSvcRule", rule)
		changed = false
	}

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for SvcSvcRule %s/%s: %v", rule.Namespace, rule.Name, err)
		if rule.Meta.EnsureErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable") {
			changed = true
		}
		flushUpdate()
		return nil
	}
	defer reader.Close()

	if rule.Meta.EnsureSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "SvcSvcRule committed to backend successfully") {
		changed = true
	}

	// Validate dependencies: ServiceFrom and ServiceTo must exist
	serviceFromID := models.NewResourceIdentifier(rule.ServiceFromRef.Name, models.WithNamespace(rule.ServiceFromRef.Namespace))
	serviceFromExists := true
	_, err = reader.GetServiceByID(ctx, serviceFromID)
	if err == ports.ErrNotFound {
		klog.Errorf("ServiceFrom %s not found for SvcSvcRule %s/%s", rule.ServiceFromRefKey(), rule.Namespace, rule.Name)
		serviceFromExists = false
	} else if err != nil {
		klog.Errorf("Failed to check ServiceFrom %s for SvcSvcRule %s/%s: %v", rule.ServiceFromRefKey(), rule.Namespace, rule.Name, err)
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check ServiceFrom %s: %v", rule.ServiceFromRefKey(), err)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceFrom validation failed") {
			changed = true
		}
		flushUpdate()
		return nil
	}

	serviceToID := models.NewResourceIdentifier(rule.ServiceToRef.Name, models.WithNamespace(rule.ServiceToRef.Namespace))
	serviceToExists := true
	_, err = reader.GetServiceByID(ctx, serviceToID)
	if err == ports.ErrNotFound {
		klog.Errorf("ServiceTo %s not found for SvcSvcRule %s/%s", rule.ServiceToRefKey(), rule.Namespace, rule.Name)
		serviceToExists = false
	} else if err != nil {
		klog.Errorf("Failed to check ServiceTo %s for SvcSvcRule %s/%s: %v", rule.ServiceToRefKey(), rule.Namespace, rule.Name, err)
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check ServiceTo %s: %v", rule.ServiceToRefKey(), err)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceTo validation failed") {
			changed = true
		}
		flushUpdate()
		return nil
	}

	// Set condition based on dependency checks
	if !serviceFromExists && !serviceToExists {
		klog.Warningf("SvcSvcRule %s/%s not ready: both ServiceFrom and ServiceTo not found", rule.Namespace, rule.Name)
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Both ServiceFrom (%s) and ServiceTo (%s) not found", rule.ServiceFromRefKey(), rule.ServiceToRefKey())) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Both referenced Services not found") {
			changed = true
		}
		if rule.Meta.EnsureValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "Dependency validation failed") {
			changed = true
		}
		flushUpdate()
		return nil
	} else if !serviceFromExists {
		klog.Warningf("SvcSvcRule %s/%s not ready: ServiceFrom %s not found", rule.Namespace, rule.Name, rule.ServiceFromRefKey())
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("ServiceFrom %s not found", rule.ServiceFromRefKey())) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceFrom not found") {
			changed = true
		}
		if rule.Meta.EnsureValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "ServiceFrom dependency missing") {
			changed = true
		}
		flushUpdate()
		return nil
	} else if !serviceToExists {
		klog.Warningf("SvcSvcRule %s/%s not ready: ServiceTo %s not found", rule.Namespace, rule.Name, rule.ServiceToRefKey())
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("ServiceTo %s not found", rule.ServiceToRefKey())) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceTo not found") {
			changed = true
		}
		if rule.Meta.EnsureValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "ServiceTo dependency missing") {
			changed = true
		}
		flushUpdate()
		return nil
	}

	// Both services exist - set Ready=True
	klog.Infof("SvcSvcRule %s/%s is ready: both ServiceFrom and ServiceTo exist", rule.Namespace, rule.Name)
	if rule.Meta.EnsureValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "SvcSvcRule passed validation") {
		changed = true
	}
	if rule.Meta.EnsureReadyCondition(metav1.ConditionTrue, models.ReasonReady, "SvcSvcRule is ready, all services exist") {
		changed = true
	}
	flushUpdate()
	return nil
}

func (cm *ConditionManager) SaveSvcSvcRuleConditions(ctx context.Context, rule *models.SvcSvcRule) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for SvcSvcRule %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncSvcSvcRules(conditionCtx, []models.SvcSvcRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync SvcSvcRule conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit SvcSvcRule conditions with ReadCommitted transaction: %w", err)
		}

		return nil
	}

	const maxRetries = 2
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			backoff := time.Duration(50*attempt) * time.Millisecond
			time.Sleep(backoff)
		}

		writer, err := cm.registry.Writer(conditionCtx)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to get writer for SvcSvcRule conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Writer creation failed on attempt %d for SvcSvcRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncSvcSvcRules(conditionCtx, []models.SvcSvcRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync SvcSvcRule with conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit SvcSvcRule conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save SvcSvcRule conditions after %d attempts", maxRetries)
}
