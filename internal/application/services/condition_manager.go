package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// IEAgAgRuleGenerator defines interface for generating IEAgAg rules from RuleS2S
type IEAgAgRuleGenerator interface {
	GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error)
}

// IEAgAgRuleManager defines interface for both generating and cleaning up IEAgAg rules
type IEAgAgRuleManager interface {
	IEAgAgRuleGenerator
	// CleanupIEAgAgRulesForRuleS2S removes all IEAgAgRules associated with a RuleS2S that became not Ready
	CleanupIEAgAgRulesForRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) error
}

// RuleS2SService interface for avoiding circular dependency with RuleS2SResourceService
type RuleS2SService interface {
	SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error
}

// ConditionManager управляет формированием условий для ресурсов ПОСЛЕ commit транзакций
type ConditionManager struct {
	registry       ports.Registry
	ieAgAgManager  IEAgAgRuleManager      // For IEAgAg rule generation and cleanup
	ruleS2SService RuleS2SService         // For proper IEAgAgRule processing with conditions and external sync
	syncManager    interfaces.SyncManager // For external sync operations to SGROUP

	// 🎯 CONDITION_BATCHING: Batching system to reduce k8s_metadata table contention
	batchMutex   sync.Mutex
	pendingBatch map[string]interface{} // resourceType:resourceKey -> resource with conditions
	batchTimer   *time.Timer
	batchSize    int
	batchTimeout time.Duration

	// 🔒 SEQUENTIAL_PROCESSING: Shared mutex for serializing condition operations to prevent deadlocks
	// This extends the NetguardFacade sequential processing pattern to cover condition batching
	sequentialMutex *sync.Mutex
}

// NewConditionManager создает новый ConditionManager
func NewConditionManager(registry ports.Registry) *ConditionManager {
	cm := &ConditionManager{
		registry:       registry,
		ieAgAgManager:  nil, // Will be injected later to avoid circular dependency
		ruleS2SService: nil, // Will be injected later to avoid circular dependency
		syncManager:    nil, // Will be injected later to avoid circular dependency

		// 🎯 CONDITION_BATCHING: Initialize batching system to reduce database contention
		pendingBatch: make(map[string]interface{}),
		batchSize:    5,               // 🔧 DEADLOCK_FIX: Reduced from 10 to 5 to minimize lock contention
		batchTimeout: 2 * time.Second, // Flush batch every 2 seconds max

		// 🔒 SEQUENTIAL_PROCESSING: Initialize without shared mutex (will be injected)
		sequentialMutex: nil,
	}
	return cm
}

// SetSequentialMutex injects the shared sequential processing mutex from NetguardFacade
// This allows condition batching to participate in the same sequential processing that prevents deadlocks
func (cm *ConditionManager) SetSequentialMutex(mutex *sync.Mutex) {
	cm.sequentialMutex = mutex
}

// SetIEAgAgRuleManager injects the IEAgAg rule manager (called after construction to avoid circular dependency)
func (cm *ConditionManager) SetIEAgAgRuleManager(manager IEAgAgRuleManager) {
	cm.ieAgAgManager = manager
}

// SetRuleS2SService injects the RuleS2S service (called after construction to avoid circular dependency)
func (cm *ConditionManager) SetRuleS2SService(service RuleS2SService) {
	cm.ruleS2SService = service
}

// SetSyncManager injects the SyncManager for external sync operations
func (cm *ConditionManager) SetSyncManager(syncManager interfaces.SyncManager) {
	cm.syncManager = syncManager
}

// SetIEAgAgRuleGenerator injects the IEAgAg rule generator (backward compatibility)
func (cm *ConditionManager) SetIEAgAgRuleGenerator(generator IEAgAgRuleGenerator) {
	// For backward compatibility - if we only get a generator, wrap it
	if manager, ok := generator.(IEAgAgRuleManager); ok {
		cm.ieAgAgManager = manager
	} else {
		// Create a wrapper that only supports generation
		cm.ieAgAgManager = &generatorOnlyWrapper{generator: generator}
	}
}

// generatorOnlyWrapper wraps IEAgAgRuleGenerator to provide IEAgAgRuleManager interface
type generatorOnlyWrapper struct {
	generator IEAgAgRuleGenerator
}

func (w *generatorOnlyWrapper) GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	return w.generator.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, ruleS2S)
}

func (w *generatorOnlyWrapper) CleanupIEAgAgRulesForRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) error {
	// No cleanup support in generator-only mode
	klog.Warningf("⚠️ IEAgAgRule cleanup not supported for RuleS2S %s/%s (generator-only mode)", ruleS2S.Namespace, ruleS2S.Name)
	return nil
}

// ProcessServiceConditions формирует условия для Service ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessServiceConditions(ctx context.Context, service *models.Service) error {
	// Очищаем старые ошибки и обновляем метаданные
	service.Meta.ClearErrorCondition()
	service.Meta.TouchOnWrite("v1")

	klog.Infof("🔄 ConditionManager.ProcessServiceConditions: processing service %s/%s after commit", service.Namespace, service.Name)

	// Получаем reader для валидации (транзакция уже закоммичена)
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to get reader for %s/%s: %v", service.Namespace, service.Name, err)
		service.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	klog.Infof("✅ ConditionManager: Setting Synced=true for %s/%s", service.Namespace, service.Name)
	service.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Service committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Проверяем базовую валидацию коммиченного объекта (без проверки существования)
	klog.Infof("🔄 ConditionManager: Validating committed service %s/%s", service.Namespace, service.Name)
	if err := serviceValidator.ValidateForCreation(ctx, *service); err != nil {
		klog.Errorf("❌ ConditionManager: Service validation failed for %s/%s: %v", service.Namespace, service.Name, err)
		service.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Service validation failed: %v", err))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service has validation errors")
		service.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	klog.Infof("✅ ConditionManager: Setting Validated=true for %s/%s", service.Namespace, service.Name)
	service.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Service passed validation")

	// Проверяем что AddressGroups РЕАЛЬНО существуют в committed состоянии
	klog.Infof("🔄 ConditionManager: Checking %d AddressGroups for %s/%s", len(service.AddressGroups), service.Namespace, service.Name)
	missingAddressGroups := []string{}
	for _, agRef := range service.AddressGroups {
		_, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{Name: agRef.Name, Namespace: agRef.Namespace})
		if err == ports.ErrNotFound {
			missingAddressGroups = append(missingAddressGroups, models.AddressGroupRefKey(agRef))
			klog.Infof("❌ ConditionManager: AddressGroup %s not found for %s/%s", models.AddressGroupRefKey(agRef), service.Namespace, service.Name)
		} else if err != nil {
			klog.Errorf("❌ ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", models.AddressGroupRefKey(agRef), service.Namespace, service.Name, err)
			service.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", models.AddressGroupRefKey(agRef), err))
			service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
			return nil
		} else {
			klog.Infof("✅ ConditionManager: AddressGroup %s found for %s/%s", models.AddressGroupRefKey(agRef), service.Namespace, service.Name)
		}
	}

	if len(missingAddressGroups) > 0 {
		klog.Errorf("❌ ConditionManager: Missing AddressGroups for %s/%s: %v", service.Namespace, service.Name, missingAddressGroups)
		service.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Missing AddressGroups: %v", missingAddressGroups))
		service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroups not found")
		return nil
	}

	// Все проверки пройдены - сервис готов
	klog.Infof("🎉 ConditionManager: All checks passed, setting Ready=true for %s/%s", service.Namespace, service.Name)
	service.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Service is ready for use")

	klog.Infof("✅ ConditionManager.ProcessServiceConditions: service %s/%s processed successfully with %d conditions", service.Namespace, service.Name, len(service.Meta.Conditions))

	// 🎯 CONDITION_BATCHING: Use batched condition updates to reduce k8s_metadata contention
	cm.batchConditionUpdate("Service", service)
	klog.V(3).Infof("🎯 CONDITION_BATCHING: Queued service %s/%s for batch condition update", service.Namespace, service.Name)

	klog.Infof("💾 ConditionManager: Successfully saved conditions for service %s/%s", service.Namespace, service.Name)
	return nil
}

// ProcessAddressGroupConditions формирует условия для AddressGroup ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
	// Очищаем старые ошибки и обновляем метаданные
	ag.Meta.ClearErrorCondition()
	ag.Meta.TouchOnWrite("v1")

	klog.Infof("🔄 ConditionManager.ProcessAddressGroupConditions: processing address group %s/%s after commit", ag.Namespace, ag.Name)

	// Получаем reader для валидации (транзакция уже закоммичена)
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to get reader for %s/%s: %v", ag.Namespace, ag.Name, err)
		ag.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader: %v", err))
		ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend reader unavailable")
		return err
	}
	defer reader.Close()

	// Создаем валидатор
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Выполняем post-commit валидацию (без проверки существования)
	klog.Infof("🔍 ConditionManager: Validating address group %s/%s after commit", ag.Namespace, ag.Name)
	if err := addressGroupValidator.ValidateForCreation(ctx, *ag); err != nil {
		klog.Errorf("❌ ConditionManager: Post-commit validation failed for %s/%s: %v", ag.Namespace, ag.Name, err)
		ag.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Post-commit validation failed: %v", err))
		ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Address group validation failed")
		return err
	}

	// 🚀 EXTERNAL_SYNC_FIX: Sync AddressGroup to SGROUP before setting Ready=True
	// This ensures external system consistency and fixes the missing AddressGroup sync
	if cm.syncManager != nil {
		klog.Infof("🔄 EXTERNAL_SYNC_FIX: Syncing AddressGroup %s/%s to SGROUP", ag.Namespace, ag.Name)
		if err := cm.syncManager.SyncEntity(ctx, ag, types.SyncOperationUpsert); err != nil {
			klog.Errorf("❌ EXTERNAL_SYNC_FIX: Failed to sync AddressGroup %s/%s to SGROUP: %v", ag.Namespace, ag.Name, err)
			ag.Meta.SetSyncedCondition(metav1.ConditionFalse, models.ReasonSyncFailed, fmt.Sprintf("Failed to sync with external SGROUP: %v", err))
			ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "External sync failed")
			ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")

			// 🎯 CONDITION_BATCHING: Even failed syncs need condition updates
			cm.batchConditionUpdate("AddressGroup", ag)
			return fmt.Errorf("external sync failed for AddressGroup %s/%s: %w", ag.Namespace, ag.Name, err)
		}
		klog.Infof("✅ EXTERNAL_SYNC_FIX: Successfully synced AddressGroup %s/%s to SGROUP", ag.Namespace, ag.Name)
	} else {
		klog.Warningf("⚠️  EXTERNAL_SYNC_FIX: SyncManager is nil, skipping external sync for AddressGroup %s/%s", ag.Namespace, ag.Name)
	}

	// Все проверки прошли успешно - устанавливаем позитивные условия
	klog.Infof("✅ ConditionManager: Setting success conditions for address group %s/%s", ag.Namespace, ag.Name)
	ag.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Address group is ready and operational")
	ag.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Address group successfully synced to backend and SGROUP")
	ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")

	klog.Infof("✅ ConditionManager.ProcessAddressGroupConditions: address group %s/%s processed successfully with %d conditions", ag.Namespace, ag.Name, len(ag.Meta.Conditions))

	// 🎯 CONDITION_BATCHING: Use batched condition updates to reduce k8s_metadata contention
	cm.batchConditionUpdate("AddressGroup", ag)
	klog.V(3).Infof("🎯 CONDITION_BATCHING: Queued AddressGroup %s/%s for batch condition update", ag.Namespace, ag.Name)

	klog.Infof("💾 ConditionManager: Successfully saved conditions for address group %s/%s", ag.Namespace, ag.Name)
	return nil
}

// ProcessRuleS2SConditions формирует условия для RuleS2S ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	// Очищаем старые ошибки и обновляем метаданные
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessRuleS2SConditions: processing rule %s/%s after commit", rule.Namespace, rule.Name)

	// 🔧 CROSS-RULES2S FIX: Use ReadCommitted reader to see recently committed binding deletions
	// This ensures the condition manager can see deleted AddressGroupBindings immediately
	// replacing the previous 10ms sleep hack with proper transaction isolation
	reader, err := cm.registry.ReaderWithReadCommitted(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get ReadCommitted reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "RuleS2S committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Проверяем базовую валидацию коммиченного объекта (без проверки существования)
	if err := ruleValidator.ValidateForCreation(ctx, *rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("RuleS2S validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "RuleS2S has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))

		// 🎯 CONDITION_BATCHING: Use batched condition updates for validation failures
		cm.batchConditionUpdate("RuleS2S", rule)
		klog.V(3).Infof("🎯 CONDITION_BATCHING: Queued failed validation RuleS2S %s/%s for batch condition update", rule.Namespace, rule.Name)
		klog.Infof("✅ ConditionManager: Saved failure conditions for RuleS2S %s/%s (basic validation failed)", rule.Namespace, rule.Name)
		return nil
	}

	// Устанавливаем Validated = true
	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "RuleS2S passed validation")

	// Проверяем существование связанных ServiceAlias в РЕАЛЬНОМ состоянии
	if err := cm.validateServiceAliasReferences(ctx, reader, rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("ServiceAlias dependency error: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Traffic direction binding validation failed")

		// 🧹 CLEANUP TRIGGER: When RuleS2S becomes Ready=False due to validation failure, clean up associated IEAgAgRules
		klog.Infof("🧹 CLEANUP_TRIGGER: RuleS2S %s/%s Ready=False (validation failed), triggering IEAgAgRule cleanup", rule.Namespace, rule.Name)
		if cm.ieAgAgManager != nil {
			if cleanupErr := cm.ieAgAgManager.CleanupIEAgAgRulesForRuleS2S(ctx, *rule); cleanupErr != nil {
				klog.Errorf("❌ ConditionManager: Failed to cleanup IEAgAgRules for failed validation RuleS2S %s/%s: %v", rule.Namespace, rule.Name, cleanupErr)
				rule.Meta.SetErrorCondition(models.ReasonCleanupError, fmt.Sprintf("Failed to cleanup IEAgAgRules: %v", cleanupErr))
				// Continue processing - don't fail condition update due to cleanup errors
			} else {
				klog.Infof("✅ ConditionManager: Successfully cleaned up IEAgAgRules for failed validation RuleS2S %s/%s", rule.Namespace, rule.Name)
			}
		} else {
			klog.Warningf("⚠️ ConditionManager: IEAgAgManager is nil, cannot cleanup rules for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}

		// 🎯 CONDITION_BATCHING: Use batched condition updates for validation failures
		cm.batchConditionUpdate("RuleS2S", rule)
		klog.V(3).Infof("🎯 CONDITION_BATCHING: Queued failed validation RuleS2S %s/%s for batch condition update", rule.Namespace, rule.Name)
		klog.Infof("✅ ConditionManager: Saved failure conditions for RuleS2S %s/%s (enhanced validation failed: %v)", rule.Namespace, rule.Name, err)
		return nil
	}

	// 🚀 READY=TRUE AS GENERATION SIGNAL: Set Ready=True first based on validation, then use as IEAgAg generation trigger
	// This breaks the circular dependency where Ready=True depends on IEAgAg existence

	// If we reach this point, all validation passed, so RuleS2S can generate IEAgAg rules
	canGenerateIEAgAg := true

	if canGenerateIEAgAg {
		// All dependencies exist - RuleS2S is Ready=True
		klog.Infof("✅ ConditionManager: RuleS2S %s/%s is Ready=True (all dependencies exist)", rule.Namespace, rule.Name)
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "RuleS2S is ready, all dependencies validated")

		// 🎯 CONDITION_BATCHING: Queue Ready=True conditions for batch update before IEAgAgRule generation
		// This ensures the aggregation system can see the Ready=True status in the database
		klog.Infof("💾 CONDITION_BATCHING: Queuing Ready=True conditions for batch update for RuleS2S %s/%s", rule.Namespace, rule.Name)
		cm.batchConditionUpdate("RuleS2S", rule)
		// Force flush batch to ensure Ready=True is visible before IEAgAg generation
		cm.flushConditionBatch()
		klog.Infof("✅ CONDITION_BATCHING: Successfully flushed Ready=True conditions for RuleS2S %s/%s", rule.Namespace, rule.Name)
		klog.Infof("✅ TIMING_FIX: Successfully saved Ready=True conditions, now database reflects correct status")

		// 🎯 GENERATION SIGNAL: Now use Ready=True to trigger IEAgAg rule generation
		klog.Infof("🚀 GENERATION_SIGNAL: RuleS2S %s/%s Ready=True, triggering IEAgAg rule generation", rule.Namespace, rule.Name)

		// Generate IEAgAg rules using the resource service
		var ieAgAgRules []models.IEAgAgRule
		if cm.ieAgAgManager != nil {
			var err error
			ieAgAgRules, err = cm.ieAgAgManager.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, *rule)
			if err != nil {
				klog.Errorf("❌ ConditionManager: Failed to generate IEAgAgRules for Ready RuleS2S %s/%s: %v", rule.Namespace, rule.Name, err)
				rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to generate IEAgAgRules: %v", err))
				// Keep Ready=True but log the generation failure
			} else {
				klog.Infof("🔨 ConditionManager: Generated %d IEAgAgRules for Ready RuleS2S %s/%s", len(ieAgAgRules), rule.Namespace, rule.Name)

				// 🎯 CRITICAL FIX: Use proper service delegation for IEAgAgRule processing
				// This ensures conditions and external sync are handled correctly
				if len(ieAgAgRules) > 0 && cm.ruleS2SService != nil {
					klog.Infof("💾 CONDITION_MANAGER_FIX: Processing %d generated IEAgAgRules via proper service for RuleS2S %s/%s", len(ieAgAgRules), rule.Namespace, rule.Name)

					// Use the proper service which handles database save + conditions + external sync
					if syncErr := cm.ruleS2SService.SyncIEAgAgRules(ctx, ieAgAgRules, ports.EmptyScope{}); syncErr != nil {
						klog.Errorf("❌ ConditionManager: Failed to process IEAgAgRules via service for RuleS2S %s/%s: %v", rule.Namespace, rule.Name, syncErr)
						rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to process IEAgAgRules: %v", syncErr))
					} else {
						klog.Infof("✅ CONDITION_MANAGER_FIX: Successfully processed %d IEAgAgRules with conditions and external sync for RuleS2S %s/%s", len(ieAgAgRules), rule.Namespace, rule.Name)
					}
				} else if len(ieAgAgRules) == 0 {
					klog.Infof("ℹ️ CONDITION_MANAGER_FIX: No IEAgAgRules to process for RuleS2S %s/%s", rule.Namespace, rule.Name)
				} else {
					klog.Warningf("⚠️ CONDITION_MANAGER_FIX: RuleS2SService is nil, cannot process IEAgAgRules for RuleS2S %s/%s", rule.Namespace, rule.Name)
				}
			}
		} else {
			// Fallback for nil generator (shouldn't happen in production)
			klog.Warningf("⚠️ ConditionManager: IEAgAgGenerator is nil for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}
	} else {
		// Dependencies missing - RuleS2S is not ready
		klog.Infof("⚠️ ConditionManager: RuleS2S %s/%s is not ready (missing dependencies)", rule.Namespace, rule.Name)
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Missing dependencies for IEAgAg rule generation")

		// 🧹 CLEANUP TRIGGER: When RuleS2S becomes Ready=False, clean up associated IEAgAgRules
		klog.Infof("🧹 CLEANUP_TRIGGER: RuleS2S %s/%s Ready=False, triggering IEAgAgRule cleanup", rule.Namespace, rule.Name)
		if cm.ieAgAgManager != nil {
			if err := cm.ieAgAgManager.CleanupIEAgAgRulesForRuleS2S(ctx, *rule); err != nil {
				klog.Errorf("❌ ConditionManager: Failed to cleanup IEAgAgRules for not-ready RuleS2S %s/%s: %v", rule.Namespace, rule.Name, err)
				rule.Meta.SetErrorCondition(models.ReasonCleanupError, fmt.Sprintf("Failed to cleanup IEAgAgRules: %v", err))
				// Continue processing - don't fail condition update due to cleanup errors
			} else {
				klog.Infof("✅ ConditionManager: Successfully cleaned up IEAgAgRules for not-ready RuleS2S %s/%s", rule.Namespace, rule.Name)
			}
		} else {
			klog.Warningf("⚠️ ConditionManager: IEAgAgManager is nil, cannot cleanup rules for RuleS2S %s/%s", rule.Namespace, rule.Name)
		}
	}

	klog.V(4).Infof("ConditionManager.ProcessRuleS2SConditions: rule %s/%s processed, checking if conditions need saving...", rule.Namespace, rule.Name)

	// 🎯 CONDITION_BATCHING: Only queue conditions here if Ready=False (Ready=True already flushed above)
	// This avoids double-batching conditions for Ready=True case while ensuring Ready=False is queued
	if !rule.Meta.IsReady() {
		klog.Infof("💾 CONDITION_BATCHING: Queuing Ready=False conditions for RuleS2S %s/%s", rule.Namespace, rule.Name)
		cm.batchConditionUpdate("RuleS2S", rule)
	} else {
		klog.Infof("✅ CONDITION_BATCHING: Skipping condition queue for Ready=True RuleS2S %s/%s (already flushed before generation)", rule.Namespace, rule.Name)
	}

	klog.Infof("✅ ConditionManager: Successfully processed and saved conditions for RuleS2S %s/%s", rule.Namespace, rule.Name)
	return nil
}

// ProcessIEAgAgRuleConditions формирует условия для IEAgAgRule ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	// 🔍 IEAGAG_CONDITION_DEBUG: Enhanced debugging for IEAgAgRule condition processing
	klog.Infof("🔍 IEAGAG_CONDITIONS: Starting condition processing for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	klog.Infof("   - Traffic: %s", rule.Traffic)
	klog.Infof("   - LocalAG: %s/%s", rule.AddressGroupLocal.Namespace, rule.AddressGroupLocal.Name)
	klog.Infof("   - TargetAG: %s/%s", rule.AddressGroup.Namespace, rule.AddressGroup.Name)
	klog.Infof("   - Current conditions count: %d", len(rule.Meta.Conditions))

	// Очищаем старые ошибки и обновляем метаданные
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("❌ IEAGAG_CONDITIONS: Failed to get reader for %s/%s: %v", rule.Namespace, rule.Name, err)
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	klog.Infof("✅ IEAGAG_CONDITIONS: Setting Synced=True for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "IEAgAgRule committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Проверяем базовую валидацию коммиченного объекта
	klog.Infof("🔄 IEAGAG_CONDITIONS: Validating IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	if err := ruleValidator.ValidateForCreation(ctx, *rule); err != nil {
		klog.Errorf("❌ IEAGAG_CONDITIONS: Validation failed for %s/%s: %v", rule.Namespace, rule.Name, err)
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("IEAgAgRule validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "IEAgAgRule has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	klog.Infof("✅ IEAGAG_CONDITIONS: Validation passed for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "IEAgAgRule passed validation")

	// Проверяем существование связанных AddressGroups
	localAGExists := true
	targetAGExists := true

	if _, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
		Name:      rule.AddressGroupLocal.Name,
		Namespace: rule.AddressGroupLocal.Namespace,
	}); err != nil {
		localAGExists = false
		klog.Warningf("⚠️ IEAGAG_CONDITIONS: Local AddressGroup %s/%s not found for IEAgAgRule %s/%s",
			rule.AddressGroupLocal.Namespace, rule.AddressGroupLocal.Name, rule.Namespace, rule.Name)
	}

	if _, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{
		Name:      rule.AddressGroup.Name,
		Namespace: rule.AddressGroup.Namespace,
	}); err != nil {
		targetAGExists = false
		klog.Warningf("⚠️ IEAGAG_CONDITIONS: Target AddressGroup %s/%s not found for IEAgAgRule %s/%s",
			rule.AddressGroup.Namespace, rule.AddressGroup.Name, rule.Namespace, rule.Name)
	}

	if !localAGExists || !targetAGExists {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, "Referenced AddressGroups not found")
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Missing AddressGroup dependencies")
		klog.Infof("⚠️ IEAGAG_CONDITIONS: IEAgAgRule %s/%s not ready due to missing AddressGroups", rule.Namespace, rule.Name)
	} else {
		// Все проверки прошли успешно - устанавливаем позитивные условия
		klog.Infof("✅ IEAGAG_CONDITIONS: Setting Ready=True for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "IEAgAgRule is ready and operational")
	}

	klog.Infof("🔄 IEAGAG_CONDITIONS: Saving conditions for IEAgAgRule %s/%s (conditions count: %d)",
		rule.Namespace, rule.Name, len(rule.Meta.Conditions))

	// 🎯 CONDITION_BATCHING: Queue conditions for batch update (non-blocking)
	cm.batchConditionUpdate("IEAgAgRule", rule)
	klog.V(3).Infof("🎯 CONDITION_BATCHING: Queued IEAgAgRule %s/%s for batch condition update", rule.Namespace, rule.Name)

	klog.Infof("✅ IEAGAG_CONDITIONS: Successfully processed and saved conditions for IEAgAgRule %s/%s", rule.Namespace, rule.Name)
	return nil
}

// ProcessAddressGroupBindingConditions формирует условия для AddressGroupBinding ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	// Очищаем старые ошибки и обновляем метаданные
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	klog.Infof("🔄 ConditionManager.ProcessAddressGroupBindingConditions: processing binding %s/%s after commit", binding.Namespace, binding.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBinding committed to backend successfully")
	klog.Infof("🔄 Step 1: Set Synced condition for binding %s/%s", binding.Namespace, binding.Name)

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Проверяем базовую валидацию коммиченного объекта (без проверки существования)
	klog.Infof("🔄 Step 2: Starting validation for binding %s/%s", binding.Namespace, binding.Name)
	if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
		klog.Errorf("❌ Step 2: Validation failed for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}
	klog.Infof("✅ Step 2: Validation passed for binding %s/%s", binding.Namespace, binding.Name)

	// Устанавливаем Validated = true
	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBinding passed validation")

	// Проверяем что Service РЕАЛЬНО существует в committed состоянии
	// Create ResourceIdentifier from ObjectReference
	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.Namespace))
	klog.Infof("🔄 Step 3: Checking service %s exists", serviceID.Key())
	service, err := reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ Step 3: Service %s not found", binding.ServiceRefKey())
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", binding.ServiceRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		klog.Errorf("❌ Step 3: Failed to get Service %s: %v", binding.ServiceRefKey(), err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", binding.ServiceRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}
	klog.Infof("✅ Step 3: Service %s found", serviceID.Key())

	// Проверяем что AddressGroup РЕАЛЬНО существует в committed состоянии
	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	klog.Infof("🔄 Step 4: Checking address group %s exists", agID.Key())
	_, err = reader.GetAddressGroupByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ Step 4: AddressGroup %s not found", binding.AddressGroupRefKey())
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", binding.AddressGroupRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("❌ Step 4: Failed to get AddressGroup %s: %v", binding.AddressGroupRefKey(), err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", binding.AddressGroupRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}
	klog.Infof("✅ Step 4: AddressGroup %s found", agID.Key())

	// Проверяем port overlaps в РЕАЛЬНОМ состоянии
	klog.Infof("🔄 Step 5: Checking port overlaps for binding %s/%s", binding.Namespace, binding.Name)
	if err := validation.CheckPortOverlaps(*service, models.AddressGroupPortMapping{}); err != nil {
		klog.Errorf("❌ Step 5: Port overlap detected for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Port overlap detected: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port conflicts detected")
		return nil
	}
	klog.Infof("✅ Step 5: No port overlaps detected for binding %s/%s", binding.Namespace, binding.Name)

	// Проверяем что AddressGroupPortMapping РЕАЛЬНО создан
	klog.Infof("🔄 Step 6: Checking port mapping %s exists", agID.Key())
	portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ Step 6: AddressGroupPortMapping %s not found", agID.Key())
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, "AddressGroupPortMapping not created")
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping was not created")
		return nil
	} else if err != nil {
		klog.Errorf("❌ Step 6: Failed to get port mapping %s: %v", agID.Key(), err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to verify port mapping: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping verification failed")
		return nil
	}
	klog.Infof("✅ Step 6: Port mapping %s found with %d access ports", agID.Key(), len(portMapping.AccessPorts))

	// Все проверки пройдены - binding готов
	accessPortsCount := len(portMapping.AccessPorts)
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupBinding is ready, %d access ports configured", accessPortsCount))

	klog.Infof("✅ ConditionManager.ProcessAddressGroupBindingConditions: binding %s/%s processed successfully with 3 conditions", binding.Namespace, binding.Name)

	// Save the processed conditions back to storage
	if err := cm.saveAddressGroupBindingConditions(ctx, binding); err != nil {
		klog.Errorf("❌ ConditionManager: Failed to save conditions for address group binding %s/%s: %v", binding.Namespace, binding.Name, err)
		return nil
	}

	return nil
}

// ProcessServiceAliasConditions формирует условия для ServiceAlias ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	// Очищаем старые ошибки и обновляем метаданные
	alias.Meta.ClearErrorCondition()
	alias.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessServiceAliasConditions: processing service alias %s/%s after commit", alias.Namespace, alias.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		alias.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	alias.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "ServiceAlias committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Проверяем базовую валидацию коммиченного объекта
	if err := aliasValidator.ValidateForCreation(ctx, alias); err != nil {
		alias.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("ServiceAlias validation failed: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceAlias has validation errors")
		alias.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	alias.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "ServiceAlias passed validation")

	// Проверяем что Service РЕАЛЬНО существует в committed состоянии
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

	// Все проверки пройдены - alias готов
	alias.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "ServiceAlias is ready and operational")

	klog.V(4).Infof("ConditionManager.ProcessServiceAliasConditions: service alias %s/%s processed successfully", alias.Namespace, alias.Name)

	// Save the processed conditions back to storage
	if err := cm.saveServiceAliasConditions(ctx, alias); err != nil {
		klog.Errorf("❌ ConditionManager: Failed to save conditions for service alias %s/%s: %v", alias.Namespace, alias.Name, err)
		return nil
	}

	return nil
}

// ProcessAddressGroupPortMappingConditions формирует условия для AddressGroupPortMapping ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	// Очищаем старые ошибки и обновляем метаданные
	mapping.Meta.ClearErrorCondition()
	mapping.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupPortMappingConditions: processing port mapping %s/%s after commit", mapping.Namespace, mapping.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	mapping.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupPortMapping committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Проверяем базовую валидацию коммиченного объекта (без проверки существования)
	if err := mappingValidator.ValidateForCreation(ctx, *mapping); err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupPortMapping validation failed: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupPortMapping has validation errors")
		mapping.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	mapping.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupPortMapping passed validation")

	// Проверяем что у mapping есть хотя бы один access port
	if len(mapping.AccessPorts) == 0 {
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "No access ports configured")
		return nil
	}

	// Проверяем что все Service, на которые ссылается mapping, РЕАЛЬНО существуют
	missingServices := []string{}
	for serviceRef := range mapping.AccessPorts {
		_, err := reader.GetServiceByID(ctx, models.ResourceIdentifier{Name: serviceRef.Name, Namespace: serviceRef.Namespace})
		if err == ports.ErrNotFound {
			missingServices = append(missingServices, models.ServiceRefKey(serviceRef))
		} else if err != nil {
			mapping.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Service %s: %v", models.ServiceRefKey(serviceRef), err))
			mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
			return nil
		}
	}

	if len(missingServices) > 0 {
		mapping.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Missing Services: %v", missingServices))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Services not found")
		return nil
	}

	// Все проверки пройдены - mapping готов
	accessPortsCount := len(mapping.AccessPorts)
	mapping.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupPortMapping is ready, %d access ports configured", accessPortsCount))

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupPortMappingConditions: port mapping %s/%s processed successfully", mapping.Namespace, mapping.Name)

	// Save the processed conditions back to storage
	if err := cm.saveAddressGroupPortMappingConditions(ctx, mapping); err != nil {
		klog.Errorf("❌ ConditionManager: Failed to save conditions for AddressGroupPortMapping %s/%s: %v", mapping.Namespace, mapping.Name, err)
		return nil
	}

	return nil
}

// ProcessAddressGroupBindingPolicyConditions формирует условия для AddressGroupBindingPolicy ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	// Очищаем старые ошибки и обновляем метаданные
	policy.Meta.ClearErrorCondition()
	policy.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupBindingPolicyConditions: processing policy %s/%s after commit", policy.Namespace, policy.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	policy.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBindingPolicy committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	// Проверяем базовую валидацию коммиченного объекта (без проверки существования)
	if err := policyValidator.ValidateForCreation(ctx, policy); err != nil {
		policy.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBindingPolicy validation failed: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBindingPolicy has validation errors")
		policy.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	policy.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBindingPolicy passed validation")

	// Проверяем что AddressGroup РЕАЛЬНО существует в committed состоянии
	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(policy.AddressGroupRef.Name, models.WithNamespace(policy.AddressGroupRef.Namespace))
	_, err = reader.GetAddressGroupByID(ctx, agID)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", policy.AddressGroupRefKey()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", policy.AddressGroupRefKey(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	// Проверяем что Service РЕАЛЬНО существует в committed состоянии
	// Create ResourceIdentifier from NamespacedObjectReference
	serviceID := models.NewResourceIdentifier(policy.ServiceRef.Name, models.WithNamespace(policy.ServiceRef.Namespace))
	_, err = reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", policy.ServiceRefKey()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", policy.ServiceRefKey(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	// Все проверки пройдены - политика готова
	policy.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "AddressGroupBindingPolicy is ready and operational")

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupBindingPolicyConditions: policy %s/%s processed successfully", policy.Namespace, policy.Name)

	// Save the processed conditions back to storage
	if err := cm.saveAddressGroupBindingPolicyConditions(ctx, policy); err != nil {
		klog.Errorf("❌ ConditionManager: Failed to save conditions for AddressGroupBindingPolicy %s/%s: %v", policy.Namespace, policy.Name, err)
		return nil
	}

	return nil
}

// ProcessNetworkConditions формирует условия для Network ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error {
	// Очищаем старые ошибки и обновляем метаданные
	network.Meta.ClearErrorCondition()
	network.Meta.TouchOnWrite("v1")

	klog.Infof("🔄 ConditionManager.ProcessNetworkConditions: processing network %s/%s after commit", network.Namespace, network.Name)

	// Получаем reader для валидации (транзакция уже закоммичена)
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to get reader for %s/%s: %v", network.Namespace, network.Name, err)
		network.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Проверяем результат синхронизации с sgroups
	if syncResult != nil {
		klog.Errorf("❌ ConditionManager: sgroups sync failed for %s/%s: %v", network.Namespace, network.Name, syncResult)
		network.Meta.SetSyncedCondition(metav1.ConditionFalse, models.ReasonSyncFailed, fmt.Sprintf("Failed to sync with sgroups: %v", syncResult))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network sync with external source failed")
		network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidating, "Validation skipped due to sync failure")
		return nil
	}

	// Backend и sgroups синхронизированы (коммит прошел успешно и sgroups тоже)
	klog.Infof("✅ ConditionManager: Setting Synced=true for %s/%s", network.Namespace, network.Name)
	network.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Network committed to backend and synced with sgroups successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	networkValidator := validator.GetNetworkValidator()

	// Проверяем только CIDR формат для уже созданного объекта
	klog.Infof("🔄 ConditionManager: Validating committed network %s/%s", network.Namespace, network.Name)
	if err := networkValidator.ValidateCIDR(network.CIDR); err != nil {
		klog.Errorf("❌ ConditionManager: Network CIDR validation failed for %s/%s: %v", network.Namespace, network.Name, err)
		network.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Network CIDR validation failed: %v", err))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network has validation errors")
		network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("CIDR validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	klog.Infof("✅ ConditionManager: Setting Validated=true for %s/%s", network.Namespace, network.Name)
	network.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Network passed validation")

	// Все проверки пройдены - сеть готова
	klog.Infof("🎉 ConditionManager: All checks passed, setting Ready=true for %s/%s", network.Namespace, network.Name)
	network.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Network is ready for use")

	klog.Infof("✅ ConditionManager.ProcessNetworkConditions: network %s/%s processed successfully with %d conditions", network.Namespace, network.Name, len(network.Meta.Conditions))

	// Save the processed conditions back to storage
	if err := cm.saveNetworkConditions(ctx, network); err != nil {
		klog.Errorf("❌ ConditionManager: Failed to save conditions for network %s/%s: %v", network.Namespace, network.Name, err)
		// Don't fail the entire operation, conditions will be reprocessed on next update
		return nil
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for network %s/%s", network.Namespace, network.Name)
	return nil
}

// ProcessNetworkBindingConditions формирует условия для NetworkBinding ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	// Очищаем старые ошибки и обновляем метаданные
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	klog.Infof("🔄 ConditionManager.ProcessNetworkBindingConditions: processing network binding %s/%s after commit", binding.Namespace, binding.Name)

	// Получаем reader для валидации (транзакция уже закоммичена)
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to get reader for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	klog.Infof("✅ ConditionManager: Setting Synced=true for %s/%s", binding.Namespace, binding.Name)
	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "NetworkBinding committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	// Проверяем базовую валидацию коммиченного объекта
	klog.Infof("🔄 ConditionManager: Validating committed network binding %s/%s", binding.Namespace, binding.Name)
	if err := bindingValidator.ValidateForCreation(ctx, *binding); err != nil {
		klog.Errorf("❌ ConditionManager: NetworkBinding validation failed for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("NetworkBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "NetworkBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	klog.Infof("✅ ConditionManager: Setting Validated=true for %s/%s", binding.Namespace, binding.Name)
	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "NetworkBinding passed validation")

	// Проверяем что Network и AddressGroup РЕАЛЬНО существуют в committed состоянии
	klog.Infof("🔄 ConditionManager: Checking Network and AddressGroup references for %s/%s", binding.Namespace, binding.Name)

	// Проверяем Network
	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetNetworkByID(ctx, networkID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ ConditionManager: Network %s not found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Network %s not found", networkID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Network not found")
		return nil
	} else if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to check Network %s for %s/%s: %v", networkID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Network %s: %v", networkID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network validation failed")
		return nil
	} else {
		klog.Infof("✅ ConditionManager: Network %s found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
	}

	// Проверяем AddressGroup
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetAddressGroupByID(ctx, addressGroupID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ ConditionManager: AddressGroup %s not found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", addressGroupID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("❌ ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", addressGroupID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", addressGroupID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	} else {
		klog.Infof("✅ ConditionManager: AddressGroup %s found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
	}

	// Все проверки пройдены - binding готов
	klog.Infof("🎉 ConditionManager: All checks passed, setting Ready=true for %s/%s", binding.Namespace, binding.Name)
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "NetworkBinding is ready for use")

	klog.Infof("✅ ConditionManager.ProcessNetworkBindingConditions: network binding %s/%s processed successfully with %d conditions", binding.Namespace, binding.Name, len(binding.Meta.Conditions))
	return nil
}

// validateServiceAliasReferences проверяет существование ServiceAlias в РЕАЛЬНОМ состоянии
// validateServicesHaveAddressGroups checks that both services have AddressGroups (following k8s-controller pattern)
func (cm *ConditionManager) validateServicesHaveAddressGroups(ctx context.Context, reader ports.Reader, rule *models.RuleS2S, localServiceID, targetServiceID models.ResourceIdentifier) error {
	klog.Infof("🔍 validateServicesHaveAddressGroups: Starting validation for RuleS2S %s/%s (following k8s-controller pattern)", rule.Namespace, rule.Name)
	klog.Infof("🔍 validateServicesHaveAddressGroups: LocalService=%s, TargetService=%s", localServiceID.Key(), targetServiceID.Key())

	// Get the actual Service objects (following k8s-controller pattern)
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return fmt.Errorf("failed to get local service '%s': %v", localServiceID.Key(), err)
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return fmt.Errorf("failed to get target service '%s': %v", targetServiceID.Key(), err)
	}

	// Check AddressGroups on services (following k8s-controller pattern)
	var inactiveConditions []string

	// Get address groups from services (like k8s-controller: localService.AddressGroups.Items)
	localAddressGroupsCount := len(localService.AddressGroups)
	targetAddressGroupsCount := len(targetService.AddressGroups)

	klog.Infof("🔍 validateServicesHaveAddressGroups: LocalService %s has %d AddressGroups", localServiceID.Key(), localAddressGroupsCount)
	klog.Infof("🔍 validateServicesHaveAddressGroups: TargetService %s has %d AddressGroups", targetServiceID.Key(), targetAddressGroupsCount)

	// Check address groups (following k8s-controller logic exactly)
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

	// If there are any inactive conditions, the RuleS2S should be marked as not ready
	if len(inactiveConditions) > 0 {
		klog.Errorf("❌ validateServicesHaveAddressGroups: RuleS2S %s/%s has inactive conditions: %s", rule.Namespace, rule.Name, strings.Join(inactiveConditions, "; "))
		return fmt.Errorf("rule is invalid due to missing address groups: %s", strings.Join(inactiveConditions, "; "))
	}

	klog.Infof("✅ validateServicesHaveAddressGroups: All services have AddressGroups - RuleS2S %s/%s is valid", rule.Namespace, rule.Name)
	return nil
}

func (cm *ConditionManager) validateServiceAliasReferences(ctx context.Context, reader ports.Reader, rule *models.RuleS2S) error {
	klog.Infof("🔍 validateServiceAliasReferences: Starting validation for RuleS2S %s/%s", rule.Namespace, rule.Name)

	// Проверяем локальный ServiceAlias
	localAliasID := models.NewResourceIdentifier(rule.ServiceLocalRef.Name, models.WithNamespace(rule.ServiceLocalRef.Namespace))
	klog.Infof("🔍 validateServiceAliasReferences: [1/4] Checking local ServiceAlias %s", localAliasID.Key())
	localAlias, err := reader.GetServiceAliasByID(ctx, localAliasID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ validateServiceAliasReferences: [1/4] Local ServiceAlias %s NOT FOUND", localAliasID.Key())
		return fmt.Errorf("local service alias '%s' not found", localAliasID.Key())
	} else if err != nil {
		klog.Errorf("❌ validateServiceAliasReferences: [1/4] Failed to get local ServiceAlias %s: %v", localAliasID.Key(), err)
		return fmt.Errorf("failed to get local service alias '%s': %v", localAliasID.Key(), err)
	}
	klog.Infof("✅ validateServiceAliasReferences: [1/4] Local ServiceAlias %s found", localAliasID.Key())

	// Проверяем целевой ServiceAlias
	targetAliasID := models.NewResourceIdentifier(rule.ServiceRef.Name, models.WithNamespace(rule.ServiceRef.Namespace))
	klog.Infof("🔍 validateServiceAliasReferences: [2/4] Checking target ServiceAlias %s", targetAliasID.Key())
	targetAlias, err := reader.GetServiceAliasByID(ctx, targetAliasID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ validateServiceAliasReferences: [2/4] Target ServiceAlias %s NOT FOUND", targetAliasID.Key())
		return fmt.Errorf("target service alias '%s' not found", targetAliasID.Key())
	} else if err != nil {
		klog.Errorf("❌ validateServiceAliasReferences: [2/4] Failed to get target ServiceAlias %s: %v", targetAliasID.Key(), err)
		return fmt.Errorf("failed to get target service alias '%s': %v", targetAliasID.Key(), err)
	}
	klog.Infof("✅ validateServiceAliasReferences: [2/4] Target ServiceAlias %s found", targetAliasID.Key())

	// Проверяем что Service, на которые ссылаются ServiceAlias, РЕАЛЬНО существуют
	localServiceID := models.NewResourceIdentifier(localAlias.ServiceRef.Name, models.WithNamespace(localAlias.Namespace))
	klog.Infof("🔍 validateServiceAliasReferences: [3/4] Checking local Service %s (referenced by ServiceAlias %s)", localServiceID.Key(), localAlias.Key())
	_, err = reader.GetServiceByID(ctx, localServiceID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ validateServiceAliasReferences: [3/4] Local Service %s NOT FOUND (referenced by ServiceAlias %s)", localServiceID.Key(), localAlias.Key())
		return fmt.Errorf("local service '%s' (referenced by ServiceAlias '%s') not found", localServiceID.Key(), localAlias.Key())
	} else if err != nil {
		klog.Errorf("❌ validateServiceAliasReferences: [3/4] Failed to get local Service %s: %v", localServiceID.Key(), err)
		return fmt.Errorf("failed to get local service '%s': %v", localServiceID.Key(), err)
	}
	klog.Infof("✅ validateServiceAliasReferences: [3/4] Local Service %s found", localServiceID.Key())

	targetServiceID := models.NewResourceIdentifier(targetAlias.ServiceRef.Name, models.WithNamespace(targetAlias.Namespace))
	klog.Infof("🔍 validateServiceAliasReferences: [4/4] Checking target Service %s (referenced by ServiceAlias %s)", targetServiceID.Key(), targetAlias.Key())
	_, err = reader.GetServiceByID(ctx, targetServiceID)
	if err == ports.ErrNotFound {
		klog.Errorf("❌ validateServiceAliasReferences: [4/4] Target Service %s NOT FOUND (referenced by ServiceAlias %s)", targetServiceID.Key(), targetAlias.Key())
		return fmt.Errorf("target service '%s' (referenced by ServiceAlias '%s') not found", targetServiceID.Key(), targetAlias.Key())
	} else if err != nil {
		klog.Errorf("❌ validateServiceAliasReferences: [4/4] Failed to get target Service %s: %v", targetServiceID.Key(), err)
		return fmt.Errorf("failed to get target service '%s': %v", targetServiceID.Key(), err)
	}
	klog.Infof("✅ validateServiceAliasReferences: [4/4] Target Service %s found", targetServiceID.Key())

	// 🆕 CORRECT VALIDATION: Check if services have AddressGroups (following k8s-controller pattern)
	if err := cm.validateServicesHaveAddressGroups(ctx, reader, rule, localServiceID, targetServiceID); err != nil {
		klog.Errorf("❌ validateServiceAliasReferences: [5/5] Service AddressGroups validation FAILED for RuleS2S %s/%s: %v", rule.Namespace, rule.Name, err)
		return fmt.Errorf("service AddressGroups validation failed: %v", err)
	}
	klog.Infof("✅ validateServiceAliasReferences: [5/5] Service AddressGroups validated successfully")

	klog.Infof("✅ validateServiceAliasReferences: All dependencies validated successfully for RuleS2S %s/%s", rule.Namespace, rule.Name)
	return nil
}

// SetDefaultConditions устанавливает начальные условия для нового ресурса ПЕРЕД созданием
func (cm *ConditionManager) SetDefaultConditions(resource interface{}) {
	switch r := resource.(type) {
	case *models.Service:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Service is being processed")

	case *models.AddressGroup:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroup is being processed")

	case *models.RuleS2S:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "RuleS2S is being processed")

	case *models.AddressGroupBinding:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupBinding is being processed")

	case *models.AddressGroupPortMapping:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupPortMapping is being processed")

	case *models.ServiceAlias:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "ServiceAlias is being processed")

	case *models.AddressGroupBindingPolicy:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "AddressGroupBindingPolicy is being processed")

	case *models.IEAgAgRule:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "IEAgAgRule is being processed")

	case *models.Network:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "Network is being processed")

	case *models.NetworkBinding:
		r.Meta.TouchOnCreate()
		r.Meta.SetValidatedCondition(metav1.ConditionUnknown, models.ReasonPending, "Validation pending")
		r.Meta.SetSyncedCondition(metav1.ConditionUnknown, models.ReasonPending, "Synchronization pending")
		r.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "NetworkBinding is being processed")
	}
}

// saveResourceConditions сохраняет conditions для любого ресурса
func (cm *ConditionManager) saveResourceConditions(ctx context.Context, resource interface{}) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	switch r := resource.(type) {
	case *models.Service:
		if err = writer.SyncServices(ctx, []models.Service{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.AddressGroup:
		if err = writer.SyncAddressGroups(ctx, []models.AddressGroup{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.RuleS2S:
		if err = writer.SyncRuleS2S(ctx, []models.RuleS2S{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.AddressGroupBinding:
		if err = writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.AddressGroupPortMapping:
		if err = writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.ServiceAlias:
		if err = writer.SyncServiceAliases(ctx, []models.ServiceAlias{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.AddressGroupBindingPolicy:
		if err = writer.SyncAddressGroupBindingPolicies(ctx, []models.AddressGroupBindingPolicy{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.IEAgAgRule:
		if err = writer.SyncIEAgAgRules(ctx, []models.IEAgAgRule{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.Network:
		if err = writer.SyncNetworks(ctx, []models.Network{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	case *models.NetworkBinding:
		if err = writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.ConditionOnlyOperation{}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported resource type for saving conditions")
	}

	if err = writer.Commit(); err != nil {
		return err
	}
	return nil
}

// saveNetworkConditions saves the processed conditions for a Network back to storage
func (cm *ConditionManager) saveNetworkConditions(ctx context.Context, network *models.Network) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving network conditions: %w", err)
	}

	// Create a scope for this specific network
	scope := ports.NewResourceIdentifierScope(network.ResourceIdentifier)

	// Sync the network with updated conditions
	// Note: This will only update the conditions, the main data should already be committed
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

// 🎯 CONDITION_BATCHING: Batched condition update system to reduce k8s_metadata contention
// This addresses the PostgreSQL timeout issues by reducing the number of database round trips

// batchConditionUpdate adds a resource to the pending batch for condition updates
func (cm *ConditionManager) batchConditionUpdate(resourceType string, resource interface{}) {
	cm.batchMutex.Lock()
	defer cm.batchMutex.Unlock()

	// Generate unique key for the resource
	var resourceKey string
	switch r := resource.(type) {
	case *models.Service:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.AddressGroup:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.RuleS2S:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.IEAgAgRule:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	default:
		// Fallback for other types
		resourceKey = fmt.Sprintf("%p", resource)
	}

	batchKey := fmt.Sprintf("%s:%s", resourceType, resourceKey)
	cm.pendingBatch[batchKey] = resource

	klog.V(3).Infof("🎯 CONDITION_BATCHING: Added %s to batch (size: %d/%d)", batchKey, len(cm.pendingBatch), cm.batchSize)

	// Check if we should flush the batch
	if len(cm.pendingBatch) >= cm.batchSize {
		klog.V(2).Infof("🎯 CONDITION_BATCHING: Flushing batch due to size limit (%d)", len(cm.pendingBatch))
		go cm.flushConditionBatch()
	} else if cm.batchTimer == nil {
		// Start the timeout timer if not already running
		cm.batchTimer = time.AfterFunc(cm.batchTimeout, func() {
			cm.batchMutex.Lock()
			defer cm.batchMutex.Unlock()
			if len(cm.pendingBatch) > 0 {
				klog.V(2).Infof("🎯 CONDITION_BATCHING: Flushing batch due to timeout (%d resources)", len(cm.pendingBatch))
				go cm.flushConditionBatch()
			}
		})
	}
}

// flushConditionBatch processes all pending condition updates in a single transaction
func (cm *ConditionManager) flushConditionBatch() {
	// 🔒 SEQUENTIAL_PROCESSING: Use shared mutex to prevent deadlocks in condition batching
	// This extends the NetguardFacade sequential processing pattern to k8s_metadata operations
	if cm.sequentialMutex != nil {
		cm.sequentialMutex.Lock()
		defer cm.sequentialMutex.Unlock()
		klog.V(2).Infof("🔒 DEADLOCK_FIX: Acquired sequential processing lock for condition batch")
	}

	cm.batchMutex.Lock()

	if len(cm.pendingBatch) == 0 {
		cm.batchMutex.Unlock()
		return
	}

	// Copy the batch and clear it
	currentBatch := make(map[string]interface{})
	for k, v := range cm.pendingBatch {
		currentBatch[k] = v
	}
	cm.pendingBatch = make(map[string]interface{})

	// Reset the timer
	if cm.batchTimer != nil {
		cm.batchTimer.Stop()
		cm.batchTimer = nil
	}

	cm.batchMutex.Unlock()

	// Process the batch
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	klog.V(2).Infof("🎯 CONDITION_BATCHING: Processing batch of %d condition updates (sequential processing: %v)",
		len(currentBatch), cm.sequentialMutex != nil)

	// Use WriterForConditions for ReadCommitted isolation
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			klog.Errorf("❌ CONDITION_BATCHING: Failed to get condition writer: %v", err)
			return
		}

		// Group resources by type for efficient batch processing
		services := make([]*models.Service, 0)
		addressGroups := make([]*models.AddressGroup, 0)
		ruleS2S := make([]*models.RuleS2S, 0)
		ieAgAgRules := make([]*models.IEAgAgRule, 0)

		for batchKey, resource := range currentBatch {
			resourceType := strings.Split(batchKey, ":")[0]
			switch resourceType {
			case "Service":
				if svc, ok := resource.(*models.Service); ok {
					services = append(services, svc)
				}
			case "AddressGroup":
				if ag, ok := resource.(*models.AddressGroup); ok {
					addressGroups = append(addressGroups, ag)
				}
			case "RuleS2S":
				if rule, ok := resource.(*models.RuleS2S); ok {
					ruleS2S = append(ruleS2S, rule)
				}
			case "IEAgAgRule":
				if rule, ok := resource.(*models.IEAgAgRule); ok {
					ieAgAgRules = append(ieAgAgRules, rule)
				}
			}
		}

		// 🚀 DEPENDENCY_ORDERED_SYNC: Process resources in external sync dependency order
		// Phase 1: Services (no external dependencies)
		success := true
		if len(services) > 0 {
			serviceModels := make([]models.Service, len(services))
			for i, svc := range services {
				serviceModels[i] = *svc
			}
			if err := writer.SyncServices(ctx, serviceModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("❌ CONDITION_BATCHING: Failed to batch sync %d services: %v", len(services), err)
				success = false
			} else {
				klog.V(2).Infof("✅ CONDITION_BATCHING: Successfully batched %d service condition updates", len(services))
			}
		}

		// Phase 2: AddressGroups (must be synced to SGROUP BEFORE IEAgAgRules)
		if len(addressGroups) > 0 && success {
			agModels := make([]models.AddressGroup, len(addressGroups))
			for i, ag := range addressGroups {
				agModels[i] = *ag
			}

			// 🚀 EXTERNAL_SYNC_COORDINATION: Sync AddressGroups to SGROUP first
			if cm.syncManager != nil {
				klog.Infof("🔄 DEPENDENCY_ORDERED_SYNC: External sync phase 1 - syncing %d AddressGroups to SGROUP", len(addressGroups))
				for _, ag := range addressGroups {
					if err := cm.syncManager.SyncEntity(ctx, ag, types.SyncOperationUpsert); err != nil {
						klog.Errorf("❌ DEPENDENCY_ORDERED_SYNC: Failed to sync AddressGroup %s/%s to SGROUP: %v", ag.Namespace, ag.Name, err)
						success = false
						break
					}
				}
				if success {
					klog.Infof("✅ DEPENDENCY_ORDERED_SYNC: Successfully synced %d AddressGroups to SGROUP", len(addressGroups))
				}
			}

			// Only update conditions if external sync succeeded
			if success {
				if err := writer.SyncAddressGroups(ctx, agModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
					klog.Errorf("❌ CONDITION_BATCHING: Failed to batch sync %d address groups: %v", len(addressGroups), err)
					success = false
				} else {
					klog.V(2).Infof("✅ CONDITION_BATCHING: Successfully batched %d AddressGroup condition updates", len(addressGroups))
				}
			}
		}

		if len(ruleS2S) > 0 && success {
			ruleModels := make([]models.RuleS2S, len(ruleS2S))
			for i, rule := range ruleS2S {
				ruleModels[i] = *rule
			}
			if err := writer.SyncRuleS2S(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("❌ CONDITION_BATCHING: Failed to batch sync %d RuleS2S: %v", len(ruleS2S), err)
				success = false
			} else {
				klog.V(2).Infof("✅ CONDITION_BATCHING: Successfully batched %d RuleS2S condition updates", len(ruleS2S))
			}
		}

		// Phase 4: IEAgAgRules (must be synced AFTER AddressGroups are in SGROUP)
		if len(ieAgAgRules) > 0 && success {
			ruleModels := make([]models.IEAgAgRule, len(ieAgAgRules))
			for i, rule := range ieAgAgRules {
				ruleModels[i] = *rule
			}

			klog.Infof("🔄 DEPENDENCY_ORDERED_SYNC: External sync phase 2 - syncing %d IEAgAgRules to SGROUP (after AddressGroups)", len(ieAgAgRules))

			if err := writer.SyncIEAgAgRules(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("❌ CONDITION_BATCHING: Failed to batch sync %d IEAgAgRules: %v", len(ieAgAgRules), err)
				success = false
			} else {
				klog.V(2).Infof("✅ CONDITION_BATCHING: Successfully batched %d IEAgAgRule condition updates", len(ieAgAgRules))
				klog.Infof("✅ DEPENDENCY_ORDERED_SYNC: Successfully synced %d IEAgAgRules after AddressGroups", len(ieAgAgRules))
			}
		}

		if success {
			if err := writer.Commit(); err != nil {
				klog.Errorf("❌ CONDITION_BATCHING: Failed to commit batch transaction: %v", err)
				writer.Abort()
			} else {
				klog.Infof("✅ CONDITION_BATCHING: Successfully committed batch of %d condition updates", len(currentBatch))
			}
		} else {
			writer.Abort()
		}
	} else {
		klog.Errorf("❌ CONDITION_BATCHING: WriterForConditions not available, falling back to individual updates")
		// Fallback to individual updates if batching not supported
		for batchKey, resource := range currentBatch {
			resourceType := strings.Split(batchKey, ":")[0]
			switch resourceType {
			case "Service":
				if svc, ok := resource.(*models.Service); ok {
					cm.saveServiceConditions(ctx, svc)
				}
			case "AddressGroup":
				if ag, ok := resource.(*models.AddressGroup); ok {
					cm.saveAddressGroupConditions(ctx, ag)
				}
			case "RuleS2S":
				if rule, ok := resource.(*models.RuleS2S); ok {
					// Individual batch save - already optimized through batching system
					cm.saveRuleS2SConditions(ctx, rule)
				}
			case "IEAgAgRule":
				if rule, ok := resource.(*models.IEAgAgRule); ok {
					// Individual batch save - already optimized through batching system
					cm.saveIEAgAgRuleConditions(ctx, rule)
				}
			}
		}
	}
}

// saveServiceConditions saves the processed conditions for a Service back to storage
func (cm *ConditionManager) saveServiceConditions(ctx context.Context, service *models.Service) error {
	// 🎯 PHASE_1_TRANSACTION_ISOLATION: Use WriterForConditions for ReadCommitted isolation
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for service %s/%s: %w", service.Namespace, service.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(service.ResourceIdentifier)

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

	// 🔄 FALLBACK: Use traditional Writer if WriterForConditions not available
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving service conditions: %w", err)
	}

	// Create a scope for this specific service
	scope := ports.NewResourceIdentifierScope(service.ResourceIdentifier)

	// Sync the service with updated conditions
	// Note: This will only update the conditions, the main data should already be committed
	// 🔧 PRODUCTION FIX: Use ConditionOnlyOperation to signal PostgreSQL backend to use fresh ReadCommitted transaction
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

// saveAddressGroupConditions saves the processed conditions for an AddressGroup back to storage
func (cm *ConditionManager) saveAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
	// 🎯 PHASE_1_TRANSACTION_ISOLATION: Use WriterForConditions for ReadCommitted isolation
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for AddressGroup %s/%s: %w", ag.Namespace, ag.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(ag.ResourceIdentifier)

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

	// 🔄 FALLBACK: Use traditional Writer if WriterForConditions not available
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving address group conditions: %w", err)
	}

	// Create a scope for this specific address group
	scope := ports.NewResourceIdentifierScope(ag.ResourceIdentifier)

	// Sync the address group with updated conditions
	// Note: This will only update the conditions, the main data should already be committed
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

// saveServiceAliasConditions saves the processed conditions for a ServiceAlias back to storage
func (cm *ConditionManager) saveServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for service alias conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(alias.ResourceIdentifier)

	// Sync the service alias with updated conditions
	// Note: This will only update the conditions, the main data should already be committed
	if err := writer.SyncServiceAliases(ctx, []models.ServiceAlias{*alias}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync service alias with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit service alias conditions: %w", err)
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for service alias %s/%s", alias.Namespace, alias.Name)
	return nil
}

// saveAddressGroupBindingConditions saves the processed conditions for an AddressGroupBinding back to storage
func (cm *ConditionManager) saveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for address group binding conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(binding.ResourceIdentifier)

	// Sync the address group binding with updated conditions
	// Note: This will only update the conditions, the main data should already be committed
	if err := writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync address group binding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit address group binding conditions: %w", err)
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for address group binding %s/%s", binding.Namespace, binding.Name)
	return nil
}

// saveIEAgAgRuleConditions saves the processed conditions for an IEAgAgRule back to storage
func (cm *ConditionManager) saveIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	// 🎯 BUSINESS_FLOW_FIX: Increased timeout for complex condition operations
	// Previous: 30s was good, but complex flows with many resources need more time
	// ReadCommitted isolation reduces contention, but condition processing can be complex
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	klog.V(2).Infof("🕐 TIMEOUT_FIX: Starting condition save for IEAgAgRule %s/%s with dedicated 30s timeout", rule.Namespace, rule.Name)

	// 🎯 PHASE_1_TRANSACTION_ISOLATION: Use WriterForConditions instead of Writer
	// This creates ReadCommitted transactions that don't conflict with main RepeatableRead transactions
	// Eliminates PostgreSQL serialization conflicts during condition updates

	// Use specialized condition writer with ReadCommitted isolation
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		klog.V(2).Infof("🚀 PHASE_1_FIX: Using WriterForConditions (ReadCommitted) for IEAgAgRule %s/%s", rule.Namespace, rule.Name)

		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for IEAgAgRule %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)

		// Single attempt with ReadCommitted - no retry needed due to reduced contention
		if err := writer.SyncIEAgAgRules(conditionCtx, []models.IEAgAgRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync IEAgAgRule conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit IEAgAgRule conditions with ReadCommitted transaction: %w", err)
		}

		klog.Infof("💾 PHASE_1_FIX: Successfully saved IEAgAgRule %s/%s conditions with ReadCommitted isolation", rule.Namespace, rule.Name)
		return nil
	}

	// 🔄 FALLBACK: Use traditional retry logic with RepeatableRead if WriterForConditions not available
	klog.V(2).Infof("⚠️ FALLBACK: WriterForConditions not available, using traditional retry for IEAgAgRule %s/%s", rule.Namespace, rule.Name)

	const maxRetries = 2 // Reduced retries since this is now fallback only
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Shorter backoff for fallback
			backoff := time.Duration(50*attempt) * time.Millisecond
			klog.V(2).Infof("⏳ FALLBACK: Retry attempt %d/%d for IEAgAgRule %s/%s after %v", attempt, maxRetries, rule.Namespace, rule.Name, backoff)
			time.Sleep(backoff)
		}

		writer, err := cm.registry.Writer(conditionCtx)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to get writer for IEAgAgRule conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Writer creation failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)

		// Sync the IEAgAgRule with updated conditions
		// Note: This will only update the conditions, the main data should already be committed
		if err := writer.SyncIEAgAgRules(conditionCtx, []models.IEAgAgRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync IEAgAgRule with conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Sync failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit IEAgAgRule conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Commit failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		// Success!
		klog.Infof("💾 TIMEOUT_FIX: Successfully saved conditions for IEAgAgRule %s/%s on attempt %d", rule.Namespace, rule.Name, attempt)
		return nil
	}

	// This should not be reached due to the maxRetries check above, but adding for safety
	return fmt.Errorf("failed to save IEAgAgRule conditions after %d attempts", maxRetries)
}

// saveRuleS2SConditions saves the processed conditions for a RuleS2S back to storage
func (cm *ConditionManager) saveRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	// 🎯 BUSINESS_FLOW_FIX: Increased timeout for complex condition operations
	// Previous: 30s was good, but complex flows with many resources need more time
	// ReadCommitted isolation reduces contention, but condition processing can be complex
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	klog.V(2).Infof("🕐 TIMEOUT_FIX: Starting condition save for RuleS2S %s/%s with dedicated 30s timeout", rule.Namespace, rule.Name)

	// 🎯 PHASE_1_TRANSACTION_ISOLATION: Use WriterForConditions instead of Writer
	// This creates ReadCommitted transactions that don't conflict with main RepeatableRead transactions
	// Eliminates PostgreSQL serialization conflicts during condition updates

	// Use specialized condition writer with ReadCommitted isolation
	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		klog.V(2).Infof("🚀 PHASE_1_FIX: Using WriterForConditions (ReadCommitted) for RuleS2S %s/%s", rule.Namespace, rule.Name)

		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for RuleS2S %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)

		// Single attempt with ReadCommitted - no retry needed due to reduced contention
		if err := writer.SyncRuleS2S(conditionCtx, []models.RuleS2S{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to sync RuleS2S conditions with ReadCommitted transaction: %w", err)
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			return fmt.Errorf("failed to commit RuleS2S conditions with ReadCommitted transaction: %w", err)
		}

		klog.Infof("💾 PHASE_1_FIX: Successfully saved RuleS2S %s/%s conditions with ReadCommitted isolation", rule.Namespace, rule.Name)
		return nil
	}

	// 🔄 FALLBACK: Use traditional retry logic with RepeatableRead if WriterForConditions not available
	klog.V(2).Infof("⚠️ FALLBACK: WriterForConditions not available, using traditional retry for RuleS2S %s/%s", rule.Namespace, rule.Name)

	const maxRetries = 2 // Reduced retries since this is now fallback only
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Shorter backoff for fallback
			backoff := time.Duration(50*attempt) * time.Millisecond
			klog.V(2).Infof("⏳ FALLBACK: Retry attempt %d/%d for RuleS2S %s/%s after %v", attempt, maxRetries, rule.Namespace, rule.Name, backoff)
			time.Sleep(backoff)
		}

		writer, err := cm.registry.Writer(conditionCtx)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to get writer for RuleS2S conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Writer creation failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)

		// Sync the RuleS2S with updated conditions
		if err := writer.SyncRuleS2S(conditionCtx, []models.RuleS2S{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync RuleS2S with conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Sync failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit RuleS2S conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("⚠️ TIMEOUT_FIX: Commit failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		// Success!
		klog.Infof("💾 TIMEOUT_FIX: Successfully saved conditions for RuleS2S %s/%s on attempt %d", rule.Namespace, rule.Name, attempt)
		return nil
	}

	// This should not be reached due to the maxRetries check above, but adding for safety
	return fmt.Errorf("failed to save RuleS2S conditions after %d attempts", maxRetries)
}

// saveAddressGroupPortMappingConditions saves the processed conditions for an AddressGroupPortMapping back to storage
func (cm *ConditionManager) saveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupPortMapping conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(mapping.ResourceIdentifier)

	// Sync the AddressGroupPortMapping with updated conditions
	if err := writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{*mapping}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupPortMapping with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupPortMapping conditions: %w", err)
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for AddressGroupPortMapping %s/%s", mapping.Namespace, mapping.Name)
	return nil
}

// saveAddressGroupBindingPolicyConditions saves the processed conditions for an AddressGroupBindingPolicy back to storage
func (cm *ConditionManager) saveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupBindingPolicy conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(policy.ResourceIdentifier)

	// Sync the AddressGroupBindingPolicy with updated conditions
	if err := writer.SyncAddressGroupBindingPolicies(ctx, []models.AddressGroupBindingPolicy{*policy}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupBindingPolicy with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupBindingPolicy conditions: %w", err)
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for AddressGroupBindingPolicy %s/%s", policy.Namespace, policy.Name)
	return nil
}

// saveNetworkBindingConditions saves the processed conditions for a NetworkBinding back to storage
func (cm *ConditionManager) saveNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for NetworkBinding conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(binding.ResourceIdentifier)

	// Sync the NetworkBinding with updated conditions
	if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync NetworkBinding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit NetworkBinding conditions: %w", err)
	}

	klog.Infof("💾 ConditionManager: Successfully saved conditions for NetworkBinding %s/%s", binding.Namespace, binding.Name)
	return nil
}

// checkRuleS2SCanGenerateIEAgAg checks if a RuleS2S has all necessary dependencies to generate IEAgAg rules
// This replaces the old circular dependency logic by checking dependencies directly
func (cm *ConditionManager) checkRuleS2SCanGenerateIEAgAg(ctx context.Context, reader ports.Reader, rule *models.RuleS2S) bool {
	klog.V(4).Infof("🔍 DEPENDENCY_CHECK: Starting dependency check for RuleS2S %s/%s", rule.Namespace, rule.Name)

	// Get LocalServiceAlias first
	localServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: LocalServiceAlias %s not found for RuleS2S %s/%s: %v",
			localServiceAliasID.Key(), rule.Namespace, rule.Name, err)
		return false
	}

	// Get LocalService from ServiceAlias
	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.ServiceRef.Namespace,
	}
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: LocalService %s (from alias %s) not found for RuleS2S %s/%s: %v",
			localServiceID.Key(), localServiceAliasID.Key(), rule.Namespace, rule.Name, err)
		return false
	}

	// Get TargetServiceAlias first
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}
	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: TargetServiceAlias %s not found for RuleS2S %s/%s: %v",
			targetServiceAliasID.Key(), rule.Namespace, rule.Name, err)
		return false
	}

	// Get TargetService from ServiceAlias
	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.ServiceRef.Namespace,
	}
	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: TargetService %s (from alias %s) not found for RuleS2S %s/%s: %v",
			targetServiceID.Key(), targetServiceAliasID.Key(), rule.Namespace, rule.Name, err)
		return false
	}

	// Check if both services have AddressGroups
	if len(localService.AddressGroups) == 0 {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: LocalService %s has no AddressGroups for RuleS2S %s/%s",
			localServiceID.Key(), rule.Namespace, rule.Name)
		return false
	}

	if len(targetService.AddressGroups) == 0 {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: TargetService %s has no AddressGroups for RuleS2S %s/%s",
			targetServiceID.Key(), rule.Namespace, rule.Name)
		return false
	}

	// Check if services have ports based on traffic direction
	var relevantService *models.Service
	if rule.Traffic == models.INGRESS {
		relevantService = localService
	} else {
		relevantService = targetService
	}

	if len(relevantService.IngressPorts) == 0 {
		klog.V(4).Infof("❌ DEPENDENCY_CHECK: Service %s has no IngressPorts for %s traffic in RuleS2S %s/%s",
			relevantService.Key(), rule.Traffic, rule.Namespace, rule.Name)
		return false
	}

	klog.V(4).Infof("✅ DEPENDENCY_CHECK: All dependencies satisfied for RuleS2S %s/%s (LocalService: %s, TargetService: %s, Traffic: %s)",
		rule.Namespace, rule.Name, localServiceID.Key(), targetServiceID.Key(), rule.Traffic)

	return true
}
