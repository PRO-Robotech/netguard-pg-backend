package conditions

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "IEAgAgRule committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	if err := ruleValidator.ValidateForPostCommit(ctx, *rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("IEAgAgRule validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "IEAgAgRule has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "IEAgAgRule passed validation")

	localAGExists := true
	targetAGExists := true

	if _, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
		Name:      rule.AddressGroupLocal.Name,
		Namespace: rule.AddressGroupLocal.Namespace,
	}); err != nil {
		localAGExists = false
	}

	if _, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
		Name:      rule.AddressGroup.Name,
		Namespace: rule.AddressGroup.Namespace,
	}); err != nil {
		targetAGExists = false
	}

	if !localAGExists || !targetAGExists {
		klog.Warningf("IEAgAgRule %s/%s not ready due to missing AddressGroups", rule.Namespace, rule.Name)
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, "Referenced AddressGroups not found")
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Missing AddressGroup dependencies")
	} else {
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "IEAgAgRule is ready and operational")
	}

	cm.batchConditionUpdate("IEAgAgRule", rule)
	return nil
}

func (cm *ConditionManager) SaveIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for IEAgAgRule %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncIEAgAgRules(conditionCtx, []models.IEAgAgRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync IEAgAgRule conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit IEAgAgRule conditions with ReadCommitted transaction: %w", err)
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
				return fmt.Errorf("failed to get writer for IEAgAgRule conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncIEAgAgRules(conditionCtx, []models.IEAgAgRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync IEAgAgRule with conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit IEAgAgRule conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save IEAgAgRule conditions after %d attempts", maxRetries)
}
