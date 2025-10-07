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

	batchMutex   sync.Mutex
	pendingBatch map[string]interface{} // resourceType:resourceKey -> resource with conditions
	batchTimer   *time.Timer
	batchSize    int
	batchTimeout time.Duration

	sequentialMutex *sync.Mutex
}

// NewConditionManager создает новый ConditionManager
func NewConditionManager(registry ports.Registry) *ConditionManager {
	cm := &ConditionManager{
		registry:       registry,
		ieAgAgManager:  nil, // Will be injected later to avoid circular dependency
		ruleS2SService: nil, // Will be injected later to avoid circular dependency
		syncManager:    nil, // Will be injected later to avoid circular dependency

		pendingBatch: make(map[string]interface{}),
		batchSize:    5,               // Reduced from 10 to 5 to minimize lock contention
		batchTimeout: 2 * time.Second, // Flush batch every 2 seconds max

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

// ProcessServiceConditions формирует условия для Service ПОСЛЕ успешного commit
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

	service.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Service is ready for use")
	cm.batchConditionUpdate("Service", service)
	return nil
}

// ProcessAddressGroupConditions формирует условия для AddressGroup ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
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

	if cm.syncManager != nil {
		if err := cm.syncManager.SyncEntity(ctx, ag, types.SyncOperationUpsert); err != nil {
			klog.Errorf("Failed to sync AddressGroup %s/%s to SGROUP: %v", ag.Namespace, ag.Name, err)
			ag.Meta.SetSyncedCondition(metav1.ConditionFalse, models.ReasonSyncFailed, fmt.Sprintf("Failed to sync with external SGROUP: %v", err))
			ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "External sync failed")
			ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")
			cm.batchConditionUpdate("AddressGroup", ag)
			return fmt.Errorf("external sync failed for AddressGroup %s/%s: %w", ag.Namespace, ag.Name, err)
		}
	}

	ag.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Address group is ready and operational")
	ag.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Address group successfully synced to backend and SGROUP")
	ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")
	cm.batchConditionUpdate("AddressGroup", ag)
	return nil
}

// ProcessRuleS2SConditions формирует условия для RuleS2S ПОСЛЕ успешного commit
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
				klog.Infof("🔨 ConditionManager: Generated %d IEAgAgRules for Ready RuleS2S %s/%s", len(ieAgAgRules), rule.Namespace, rule.Name)
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

// ProcessIEAgAgRuleConditions формирует условия для IEAgAgRule ПОСЛЕ успешного commit
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

// ProcessAddressGroupBindingConditions формирует условия для AddressGroupBinding ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBinding committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	if err := bindingValidator.ValidateForPostCommit(ctx, binding); err != nil {
		klog.Errorf("AddressGroupBinding validation failed for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBinding passed validation")

	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.Namespace))
	service, err := reader.GetServiceByID(ctx, serviceID)
	if err == ports.ErrNotFound {
		klog.Errorf("Service %s not found for binding %s/%s", binding.ServiceRefKey(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", binding.ServiceRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to get Service %s for binding %s/%s: %v", binding.ServiceRefKey(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", binding.ServiceRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	_, err = reader.GetAddressGroupByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("AddressGroup %s not found for binding %s/%s", binding.AddressGroupRefKey(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", binding.AddressGroupRefKey()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to get AddressGroup %s for binding %s/%s: %v", binding.AddressGroupRefKey(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", binding.AddressGroupRefKey(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	if err := validation.CheckPortOverlaps(*service, models.AddressGroupPortMapping{}); err != nil {
		klog.Errorf("Port overlap detected for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Port overlap detected: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port conflicts detected")
		return nil
	}

	portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == ports.ErrNotFound {
		klog.Errorf("AddressGroupPortMapping not created for binding %s/%s", binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, "AddressGroupPortMapping not created")
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping was not created")
		return nil
	} else if err != nil {
		klog.Errorf("Failed to verify port mapping for binding %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to verify port mapping: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping verification failed")
		return nil
	}

	accessPortsCount := len(portMapping.AccessPorts)
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupBinding is ready, %d access ports configured", accessPortsCount))

	if err := cm.saveAddressGroupBindingConditions(ctx, binding); err != nil {
		klog.Errorf("Failed to save conditions for AddressGroupBinding %s/%s: %v", binding.Namespace, binding.Name, err)
		return nil
	}

	return nil
}

// ProcessServiceAliasConditions формирует условия для ServiceAlias ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	alias.Meta.ClearErrorCondition()
	alias.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		alias.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	alias.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "ServiceAlias committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	if err := aliasValidator.ValidateForPostCommit(ctx, *alias); err != nil {
		alias.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("ServiceAlias validation failed: %v", err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "ServiceAlias has validation errors")
		alias.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	alias.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "ServiceAlias passed validation")

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

	alias.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "ServiceAlias is ready and operational")

	if err := cm.saveServiceAliasConditions(ctx, alias); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for service alias %s/%s: %v", alias.Namespace, alias.Name, err)
		return nil
	}

	return nil
}

// ProcessAddressGroupPortMappingConditions формирует условия для AddressGroupPortMapping ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	mapping.Meta.ClearErrorCondition()
	mapping.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	mapping.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupPortMapping committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	if err := mappingValidator.ValidateForPostCommit(ctx, *mapping); err != nil {
		mapping.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupPortMapping validation failed: %v", err))
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupPortMapping has validation errors")
		mapping.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	mapping.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupPortMapping passed validation")

	if len(mapping.AccessPorts) == 0 {
		mapping.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "No access ports configured")
		return nil
	}

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

	accessPortsCount := len(mapping.AccessPorts)
	mapping.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupPortMapping is ready, %d access ports configured", accessPortsCount))

	if err := cm.saveAddressGroupPortMappingConditions(ctx, mapping); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for AddressGroupPortMapping %s/%s: %v", mapping.Namespace, mapping.Name, err)
		return nil
	}

	return nil
}

// ProcessAddressGroupBindingPolicyConditions формирует условия для AddressGroupBindingPolicy ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	policy.Meta.ClearErrorCondition()
	policy.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	policy.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "AddressGroupBindingPolicy committed to backend successfully")

	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	if err := policyValidator.ValidateForPostCommit(ctx, *policy); err != nil {
		policy.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBindingPolicy validation failed: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBindingPolicy has validation errors")
		policy.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	policy.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBindingPolicy passed validation")

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

	policy.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "AddressGroupBindingPolicy is ready and operational")

	if err := cm.saveAddressGroupBindingPolicyConditions(ctx, policy); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for AddressGroupBindingPolicy %s/%s: %v", policy.Namespace, policy.Name, err)
		return nil
	}

	return nil
}

// ProcessNetworkConditions формирует условия для Network ПОСЛЕ успешного commit
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

	if err := cm.saveNetworkConditions(ctx, network); err != nil {
		klog.Errorf("ConditionManager: Failed to save conditions for network %s/%s: %v", network.Namespace, network.Name, err)
		return nil
	}

	return nil
}

// ProcessNetworkBindingConditions формирует условия для NetworkBinding ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		klog.Errorf("ConditionManager: Failed to get reader for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	binding.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "NetworkBinding committed to backend successfully")
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetNetworkBindingValidator()

	if err := bindingValidator.ValidateForPostCommit(ctx, *binding); err != nil {
		klog.Errorf("ConditionManager: NetworkBinding validation failed for %s/%s: %v", binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("NetworkBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "NetworkBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "NetworkBinding passed validation")
	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetNetworkByID(ctx, networkID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: Network %s not found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Network %s not found", networkID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Network not found")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check Network %s for %s/%s: %v", networkID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Network %s: %v", networkID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network validation failed")
		return nil
	} else {
		klog.Infof("ConditionManager: Network %s found for %s/%s", networkID.Key(), binding.Namespace, binding.Name)
	}

	// Проверяем AddressGroup
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	_, err = reader.GetAddressGroupByID(ctx, addressGroupID)
	if err == ports.ErrNotFound {
		klog.Errorf("ConditionManager: AddressGroup %s not found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", addressGroupID.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced AddressGroup not found")
		return nil
	} else if err != nil {
		klog.Errorf("ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", addressGroupID.Key(), binding.Namespace, binding.Name, err)
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", addressGroupID.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	} else {
		klog.Infof("ConditionManager: AddressGroup %s found for %s/%s", addressGroupID.Key(), binding.Namespace, binding.Name)
	}

	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "NetworkBinding is ready for use")

	return nil
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

func (cm *ConditionManager) saveNetworkConditions(ctx context.Context, network *models.Network) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving network conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(network.ResourceIdentifier)

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

	klog.V(3).Infof("Added %s to batch (size: %d/%d)", batchKey, len(cm.pendingBatch), cm.batchSize)

	if len(cm.pendingBatch) >= cm.batchSize {
		klog.V(2).Infof("Flushing batch due to size limit (%d)", len(cm.pendingBatch))
		go cm.flushConditionBatch()
	} else if cm.batchTimer == nil {
		cm.batchTimer = time.AfterFunc(cm.batchTimeout, func() {
			cm.batchMutex.Lock()
			defer cm.batchMutex.Unlock()
			if len(cm.pendingBatch) > 0 {
				klog.V(2).Infof("Flushing batch due to timeout (%d resources)", len(cm.pendingBatch))
				go cm.flushConditionBatch()
			}
		})
	}
}

func (cm *ConditionManager) flushConditionBatch() {
	if cm.sequentialMutex != nil {
		cm.sequentialMutex.Lock()
		defer cm.sequentialMutex.Unlock()
		klog.V(2).Infof("Acquired sequential processing lock for condition batch")
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			klog.Errorf("Failed to get condition writer: %v", err)
			return
		}

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

		success := true
		if len(services) > 0 {
			serviceModels := make([]models.Service, len(services))
			for i, svc := range services {
				serviceModels[i] = *svc
			}
			if err := writer.SyncServices(ctx, serviceModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d services: %v", len(services), err)
				success = false
			}
		}

		if len(addressGroups) > 0 && success {
			agModels := make([]models.AddressGroup, len(addressGroups))
			for i, ag := range addressGroups {
				agModels[i] = *ag
			}

			if cm.syncManager != nil {
				for _, ag := range addressGroups {
					if err := cm.syncManager.SyncEntity(ctx, ag, types.SyncOperationUpsert); err != nil {
						klog.Errorf("Failed to sync AddressGroup %s/%s to SGROUP: %v", ag.Namespace, ag.Name, err)
						success = false
						break
					}
				}
			}

			if success {
				if err := writer.SyncAddressGroups(ctx, agModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
					klog.Errorf("Failed to batch sync %d address groups: %v", len(addressGroups), err)
					success = false
				}
			}
		}

		if len(ruleS2S) > 0 && success {
			ruleModels := make([]models.RuleS2S, len(ruleS2S))
			for i, rule := range ruleS2S {
				ruleModels[i] = *rule
			}
			if err := writer.SyncRuleS2S(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d RuleS2S: %v", len(ruleS2S), err)
				success = false
			}
		}

		if len(ieAgAgRules) > 0 && success {
			ruleModels := make([]models.IEAgAgRule, len(ieAgAgRules))
			for i, rule := range ieAgAgRules {
				ruleModels[i] = *rule
			}

			if err := writer.SyncIEAgAgRules(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d IEAgAgRules: %v", len(ieAgAgRules), err)
				success = false
			}
		}

		if success {
			if err := writer.Commit(); err != nil {
				klog.Errorf("Failed to commit batch transaction: %v", err)
				writer.Abort()
			}
		} else {
			writer.Abort()
		}
	} else {
		klog.Errorf("WriterForConditions not available, falling back to individual updates")
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

func (cm *ConditionManager) saveServiceConditions(ctx context.Context, service *models.Service) error {
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

	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving service conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(service.ResourceIdentifier)
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

func (cm *ConditionManager) saveAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error {
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

	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for saving address group conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(ag.ResourceIdentifier)
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

func (cm *ConditionManager) saveServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for service alias conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(alias.ResourceIdentifier)
	if err := writer.SyncServiceAliases(ctx, []models.ServiceAlias{*alias}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync service alias with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit service alias conditions: %w", err)
	}

	return nil
}

func (cm *ConditionManager) saveAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for address group binding conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(binding.ResourceIdentifier)
	if err := writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync address group binding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit address group binding conditions: %w", err)
	}

	return nil
}

func (cm *ConditionManager) saveIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for IEAgAgRule %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)
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
			klog.V(2).Infof("Writer creation failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)
		if err := writer.SyncIEAgAgRules(conditionCtx, []models.IEAgAgRule{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync IEAgAgRule with conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Sync failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit IEAgAgRule conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Commit failed on attempt %d for IEAgAgRule %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save IEAgAgRule conditions after %d attempts", maxRetries)
}

func (cm *ConditionManager) saveRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	conditionCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {

		writer, err := registryWithConditions.WriterForConditions(conditionCtx)
		if err != nil {
			return fmt.Errorf("failed to get condition writer for RuleS2S %s/%s: %w", rule.Namespace, rule.Name, err)
		}

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)
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

		scope := ports.NewResourceIdentifierScope(rule.ResourceIdentifier)
		if err := writer.SyncRuleS2S(conditionCtx, []models.RuleS2S{*rule}, scope, ports.ConditionOnlyOperation{}); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to sync RuleS2S with conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Sync failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		if err := writer.Commit(); err != nil {
			writer.Abort()
			if attempt == maxRetries {
				return fmt.Errorf("failed to commit RuleS2S conditions after %d attempts: %w", maxRetries, err)
			}
			klog.V(2).Infof("Commit failed on attempt %d for RuleS2S %s/%s: %v", attempt, rule.Namespace, rule.Name, err)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save RuleS2S conditions after %d attempts", maxRetries)
}

func (cm *ConditionManager) saveAddressGroupPortMappingConditions(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupPortMapping conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(mapping.ResourceIdentifier)
	if err := writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{*mapping}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupPortMapping with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupPortMapping conditions: %w", err)
	}

	return nil
}

func (cm *ConditionManager) saveAddressGroupBindingPolicyConditions(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for AddressGroupBindingPolicy conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(policy.ResourceIdentifier)
	if err := writer.SyncAddressGroupBindingPolicies(ctx, []models.AddressGroupBindingPolicy{*policy}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync AddressGroupBindingPolicy with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit AddressGroupBindingPolicy conditions: %w", err)
	}

	return nil
}

func (cm *ConditionManager) saveNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	writer, err := cm.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for NetworkBinding conditions: %w", err)
	}

	scope := ports.NewResourceIdentifierScope(binding.ResourceIdentifier)
	if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to sync NetworkBinding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to commit NetworkBinding conditions: %w", err)
	}

	return nil
}
