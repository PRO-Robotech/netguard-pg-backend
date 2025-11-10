package conditions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.ReaderWithReadCommitted(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get ReadCommitted reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "RuleS2S committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForPostCommit(ctx, *rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("RuleS2S validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "RuleS2S has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))

		cm.batchConditionUpdate("RuleS2S", rule)
		return nil
	}

	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "RuleS2S passed validation")

	// Проверяем существование связанных ServiceAlias в РЕАЛЬНОМ состоянии
	if err := cm.validateServiceReferences(ctx, reader, rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service dependency error: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Traffic direction binding validation failed")

		klog.Infof("RuleS2S %s/%s validation failed, triggering IEAgAgRule cleanup", rule.Namespace, rule.Name)
		if cm.ieAgAgManager != nil {
			if cleanupErr := cm.ieAgAgManager.CleanupIEAgAgRulesForRuleS2S(ctx, *rule); cleanupErr != nil {
				rule.Meta.SetErrorCondition(models.ReasonCleanupError, fmt.Sprintf("Failed to cleanup IEAgAgRules: %v", cleanupErr))
			}
		} else {
			klog.Warningf("IEAgAgManager is nil, cannot cleanup rules for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}

		cm.batchConditionUpdate("RuleS2S", rule)
		return nil
	}

	canGenerateIEAgAg := true

	if canGenerateIEAgAg {
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "RuleS2S is ready, all dependencies validated")

		cm.batchConditionUpdate("RuleS2S", rule)
		cm.flushConditionBatch()

		// Generate IEAgAg rules using the resource service
		var ieAgAgRules []models.IEAgAgRule
		if cm.ieAgAgManager != nil {
			var err error
			ieAgAgRules, err = cm.ieAgAgManager.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, *rule)
			if err != nil {
				rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to generate IEAgAgRules: %v", err))
				// Keep Ready=True but log the generation failure
			} else {
				klog.Infof("ConditionManager: Generated %d IEAgAgRules for Ready RuleS2S %s/%s", len(ieAgAgRules), rule.Namespace, rule.Name)
				if len(ieAgAgRules) > 0 && cm.ruleS2SService != nil {

					if syncErr := cm.ruleS2SService.SyncIEAgAgRules(ctx, ieAgAgRules, ports.EmptyScope{}); syncErr != nil {
						rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to process IEAgAgRules: %v", syncErr))
					}
				} else if len(ieAgAgRules) == 0 {
					klog.Infof("No IEAgAgRules to process for RuleS2S %s/%s", rule.Namespace, rule.Name)
				} else {
					klog.Warningf("RuleS2SService is nil, cannot process IEAgAgRules for RuleS2S %s/%s", rule.Namespace, rule.Name)
				}
			}
		} else {
			klog.Warningf("IEAgAgGenerator is nil for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Missing dependencies for IEAgAg rule generation")

		klog.Infof("RuleS2S %s/%s not ready, triggering IEAgAgRule cleanup", rule.Namespace, rule.Name)
		if cm.ieAgAgManager != nil {
			if err := cm.ieAgAgManager.CleanupIEAgAgRulesForRuleS2S(ctx, *rule); err != nil {
				klog.Errorf("Failed to cleanup IEAgAgRules for RuleS2S %s/%s: %v", rule.Namespace, rule.Name, err)
				rule.Meta.SetErrorCondition(models.ReasonCleanupError, fmt.Sprintf("Failed to cleanup IEAgAgRules: %v", err))
			}
		} else {
			klog.Warningf("IEAgAgManager is nil, cannot cleanup rules for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}
	}

	if !rule.Meta.IsReady() {
		cm.batchConditionUpdate("RuleS2S", rule)
	}

	return nil
}

func (cm *ConditionManager) SaveRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {

		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for RuleS2S %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncRuleS2S(conditionCtx, []models.RuleS2S{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync RuleS2S conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit RuleS2S conditions with ReadCommitted transaction: %w", err)
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
				return fmt.Errorf("failed to get writer for RuleS2S conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Writer creation failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncRuleS2S(conditionCtx, []models.RuleS2S{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync RuleS2S with conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit RuleS2S conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save RuleS2S conditions after %d attempts", maxRetries)
}

func (cm *ConditionManager) validateServicesHaveAddressGroups(ctx context.Context, reader ports.Reader, rule *models.RuleS2S, localServiceID, targetServiceID models.ResourceIdentifier) error {
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return fmt.Errorf("failed to get local service '%s': %v", localServiceID.Key(), err)
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return fmt.Errorf("failed to get target service '%s': %v", targetServiceID.Key(), err)
	}

	var inactiveConditions []string
	localAddressGroupsCount := len(localService.AddressGroups)
	targetAddressGroupsCount := len(targetService.AddressGroups)
	if localAddressGroupsCount == 0 && targetAddressGroupsCount == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("Both services have no address groups: localService '%s', targetService '%s'",
				localService.Name, targetService.Name))
	} else if localAddressGroupsCount == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("LocalService '%s' has no address groups", localService.Name))
	} else if targetAddressGroupsCount == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("TargetService '%s' has no address groups", targetService.Name))
	}

	if len(inactiveConditions) > 0 {
		klog.Errorf("validateServicesHaveAddressGroups: RuleS2S %s/%s has inactive conditions: %s", rule.Namespace, rule.Name, strings.Join(inactiveConditions, "; "))
		return fmt.Errorf("rule is invalid due to missing address groups: %s", strings.Join(inactiveConditions, "; "))
	}

	return nil
}

func (cm *ConditionManager) validateServiceReferences(ctx context.Context, reader ports.Reader, rule *models.RuleS2S) error {
	localServiceID := models.NewResourceIdentifier(rule.ServiceLocalRef.Name, models.WithNamespace(rule.ServiceLocalRef.Namespace))
	_, err := reader.GetServiceByID(ctx, localServiceID)
	if err == ports.ErrNotFound {
		klog.Errorf("validateServiceReferences: [1/2] Local Service %s NOT FOUND", localServiceID.Key())
		return fmt.Errorf("local service '%s' not found", localServiceID.Key())
	} else if err != nil {
		klog.Errorf("validateServiceReferences: [1/2] Failed to get local Service %s: %v", localServiceID.Key(), err)
		return fmt.Errorf("failed to get local service '%s': %v", localServiceID.Key(), err)
	}

	targetServiceID := models.NewResourceIdentifier(rule.ServiceRef.Name, models.WithNamespace(rule.ServiceRef.Namespace))
	_, err = reader.GetServiceByID(ctx, targetServiceID)
	if err == ports.ErrNotFound {
		klog.Errorf("validateServiceReferences: [2/2] Target Service %s NOT FOUND", targetServiceID.Key())
		return fmt.Errorf("target service '%s' not found", targetServiceID.Key())
	} else if err != nil {
		klog.Errorf("validateServiceReferences: [2/2] Failed to get target Service %s: %v", targetServiceID.Key(), err)
		return fmt.Errorf("failed to get target service '%s': %v", targetServiceID.Key(), err)
	}

	if err := cm.validateServicesHaveAddressGroups(ctx, reader, rule, localServiceID, targetServiceID); err != nil {
		klog.Errorf("validateServiceReferences: [3/3] Service AddressGroups validation FAILED for RuleS2S %s/%s: %v", rule.Namespace, rule.Name, err)
		return fmt.Errorf("service AddressGroups validation failed: %v", err)
	}

	return nil
}
