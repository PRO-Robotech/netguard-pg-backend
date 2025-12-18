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

func (cm *ConditionManager) ProcessIECidrSvcRuleConditions(ctx context.Context, rule *models.IECidrSvcRule) error {
	changed := rule.Meta.ClearErrorConditionIfPresent()
	_ = changed // Used for condition tracking

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("Failed to get reader for IECidrSvcRule %s/%s: %v", rule.Namespace, rule.Name, err)
		if rule.Meta.EnsureErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable") {
			changed = true
		}
		return nil
	}
	defer reader.Close()

	if rule.Meta.EnsureSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "IECidrSvcRule committed to backend successfully") {
		changed = true
	}

	// Validate dependency: Service must exist
	serviceNamespace := rule.ServiceRef.Namespace
	if serviceNamespace == "" {
		serviceNamespace = rule.Namespace
	}
	serviceID := models.NewResourceIdentifier(rule.ServiceRef.Name, models.WithNamespace(serviceNamespace))
	serviceExists := true
	_, err = reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		klog.Errorf("Service %s/%s not found for IECidrSvcRule %s/%s", serviceNamespace, rule.ServiceRef.Name, rule.Namespace, rule.Name)
		serviceExists = false
	} else if err != nil {
		klog.Errorf("Failed to check Service %s/%s for IECidrSvcRule %s/%s: %v", serviceNamespace, rule.ServiceRef.Name, rule.Namespace, rule.Name, err)
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Service %s/%s: %v", serviceNamespace, rule.ServiceRef.Name, err)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed") {
			changed = true
		}
		return nil
	}

	// Set condition based on dependency check
	if !serviceExists {
		klog.Warningf("IECidrSvcRule %s/%s not ready: Service %s/%s not found", rule.Namespace, rule.Name, serviceNamespace, rule.ServiceRef.Name)
		if rule.Meta.EnsureErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s/%s not found", serviceNamespace, rule.ServiceRef.Name)) {
			changed = true
		}
		if rule.Meta.EnsureReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Service not found") {
			changed = true
		}
		if rule.Meta.EnsureValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, "Service dependency missing") {
			changed = true
		}
		return nil
	}

	// Service exists - set Ready=True
	klog.Infof("IECidrSvcRule %s/%s is ready: Service %s/%s exists", rule.Namespace, rule.Name, serviceNamespace, rule.ServiceRef.Name)
	if rule.Meta.EnsureValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "IECidrSvcRule passed validation") {
		changed = true
	}
	if rule.Meta.EnsureReadyCondition(metav1.ConditionTrue, models.ReasonReady, "IECidrSvcRule is ready, service exists") {
		changed = true
	}
	return nil
}

func (cm *ConditionManager) SaveIECidrSvcRuleConditions(ctx context.Context, rule *models.IECidrSvcRule) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for IECidrSvcRule %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncIECidrSvcRules(conditionCtx, []models.IECidrSvcRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync IECidrSvcRule conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit IECidrSvcRule conditions with ReadCommitted transaction: %w", err)
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
				return fmt.Errorf("failed to get writer for IECidrSvcRule conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Writer creation failed on attempt %d for IECidrSvcRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.EmptyScope{}
		if err := writer.SyncIECidrSvcRules(conditionCtx, []models.IECidrSvcRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync IECidrSvcRule with conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit IECidrSvcRule conditions after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save IECidrSvcRule conditions after %d attempts", maxRetries)
}
