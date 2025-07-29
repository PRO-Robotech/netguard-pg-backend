package services

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// ConditionManager управляет формированием условий для ресурсов ПОСЛЕ commit транзакций
type ConditionManager struct {
	registry        ports.Registry
	netguardService *NetguardService
}

// NewConditionManager создает новый ConditionManager
func NewConditionManager(registry ports.Registry, netguardService *NetguardService) *ConditionManager {
	return &ConditionManager{
		registry:        registry,
		netguardService: netguardService,
	}
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

	// Проверяем базовую валидацию коммиченного объекта
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
		_, err := reader.GetAddressGroupByID(ctx, agRef.ResourceIdentifier)
		if err == ports.ErrNotFound {
			missingAddressGroups = append(missingAddressGroups, agRef.Key())
			klog.Infof("❌ ConditionManager: AddressGroup %s not found for %s/%s", agRef.Key(), service.Namespace, service.Name)
		} else if err != nil {
			klog.Errorf("❌ ConditionManager: Failed to check AddressGroup %s for %s/%s: %v", agRef.Key(), service.Namespace, service.Name, err)
			service.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check AddressGroup %s: %v", agRef.Key(), err))
			service.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
			return nil
		} else {
			klog.Infof("✅ ConditionManager: AddressGroup %s found for %s/%s", agRef.Key(), service.Namespace, service.Name)
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

	// Выполняем post-commit валидацию
	klog.Infof("🔍 ConditionManager: Validating address group %s/%s after commit", ag.Namespace, ag.Name)
	if err := addressGroupValidator.ValidateForCreation(ctx, *ag); err != nil {
		klog.Errorf("❌ ConditionManager: Post-commit validation failed for %s/%s: %v", ag.Namespace, ag.Name, err)
		ag.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Post-commit validation failed: %v", err))
		ag.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Address group validation failed")
		return err
	}

	// Все проверки прошли успешно - устанавливаем позитивные условия
	klog.Infof("✅ ConditionManager: Setting success conditions for address group %s/%s", ag.Namespace, ag.Name)
	ag.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Address group is ready and operational")
	ag.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Address group successfully synced to backend")
	ag.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Address group passed all validations")

	klog.Infof("✅ ConditionManager.ProcessAddressGroupConditions: address group %s/%s processed successfully with %d conditions", ag.Namespace, ag.Name, len(ag.Meta.Conditions))
	return nil
}

// ProcessRuleS2SConditions формирует условия для RuleS2S ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error {
	// Очищаем старые ошибки и обновляем метаданные
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessRuleS2SConditions: processing rule %s/%s after commit", rule.Namespace, rule.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "RuleS2S committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Проверяем базовую валидацию коммиченного объекта
	if err := ruleValidator.ValidateForCreation(ctx, *rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("RuleS2S validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "RuleS2S has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "RuleS2S passed validation")

	// Проверяем существование связанных ServiceAlias в РЕАЛЬНОМ состоянии
	if err := cm.validateServiceAliasReferences(ctx, reader, rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("ServiceAlias dependency error: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required ServiceAlias not found")
		return nil
	}

	// Проверяем что IEAgAgRule правила РЕАЛЬНО сгенерированы
	ieAgAgRules, err := cm.netguardService.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, *rule)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to generate IEAgAgRules: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "IEAgAgRule generation failed")
		return nil
	}

	// Проверяем что правила созданы (не пустой список)
	if len(ieAgAgRules) == 0 {
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "No IEAgAgRules can be generated (missing AddressGroups or ports)")
		return nil
	}

	// Проверяем что IEAgAgRules РЕАЛЬНО существуют в committed состоянии
	existingIEAgAgRules := 0
	for _, ieRule := range ieAgAgRules {
		if _, err := reader.GetIEAgAgRuleByID(ctx, ieRule.ResourceIdentifier); err == nil {
			existingIEAgAgRules++
		}
	}

	if existingIEAgAgRules == 0 {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, "Generated IEAgAgRules not found in backend")
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "IEAgAgRules were not created")
		return nil
	}

	// Все проверки пройдены - правило готово
	rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("RuleS2S is ready, %d/%d IEAgAgRules created", existingIEAgAgRules, len(ieAgAgRules)))

	klog.V(4).Infof("ConditionManager.ProcessRuleS2SConditions: rule %s/%s processed successfully", rule.Namespace, rule.Name)
	return nil
}

// ProcessAddressGroupBindingConditions формирует условия для AddressGroupBinding ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error {
	// Очищаем старые ошибки и обновляем метаданные
	binding.Meta.ClearErrorCondition()
	binding.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupBindingConditions: processing binding %s/%s after commit", binding.Namespace, binding.Name)

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

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Проверяем базовую валидацию коммиченного объекта
	if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBinding has validation errors")
		binding.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	binding.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBinding passed validation")

	// Проверяем что Service РЕАЛЬНО существует в committed состоянии
	service, err := reader.GetServiceByID(ctx, binding.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", binding.ServiceRef.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", binding.ServiceRef.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	// Проверяем что AddressGroup РЕАЛЬНО существует в committed состоянии
	_, err = reader.GetAddressGroupByID(ctx, binding.AddressGroupRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", binding.AddressGroupRef.Key()))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", binding.AddressGroupRef.Key(), err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	// Проверяем port overlaps в РЕАЛЬНОМ состоянии
	if err := validation.CheckPortOverlaps(*service, models.AddressGroupPortMapping{}); err != nil {
		binding.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Port overlap detected: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port conflicts detected")
		return nil
	}

	// Проверяем что AddressGroupPortMapping РЕАЛЬНО создан
	portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, binding.AddressGroupRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, "AddressGroupPortMapping not created")
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping was not created")
		return nil
	} else if err != nil {
		binding.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to verify port mapping: %v", err))
		binding.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Port mapping verification failed")
		return nil
	}

	// Все проверки пройдены - binding готов
	accessPortsCount := len(portMapping.AccessPorts)
	binding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("AddressGroupBinding is ready, %d access ports configured", accessPortsCount))

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupBindingConditions: binding %s/%s processed successfully", binding.Namespace, binding.Name)
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
	_, err = reader.GetServiceByID(ctx, alias.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		alias.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Referenced Service %s not found", alias.ServiceRef.Key()))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Referenced Service not found")
		return nil
	} else if err != nil {
		alias.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get referenced Service %s: %v", alias.ServiceRef.Key(), err))
		alias.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	// Все проверки пройдены - alias готов
	alias.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "ServiceAlias is ready and operational")

	klog.V(4).Infof("ConditionManager.ProcessServiceAliasConditions: service alias %s/%s processed successfully", alias.Namespace, alias.Name)
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

	// Проверяем базовую валидацию коммиченного объекта
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
		_, err := reader.GetServiceByID(ctx, serviceRef.ResourceIdentifier)
		if err == ports.ErrNotFound {
			missingServices = append(missingServices, serviceRef.Key())
		} else if err != nil {
			mapping.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to check Service %s: %v", serviceRef.Key(), err))
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

	// Проверяем базовую валидацию коммиченного объекта
	if err := policyValidator.ValidateForCreation(ctx, policy); err != nil {
		policy.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("AddressGroupBindingPolicy validation failed: %v", err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroupBindingPolicy has validation errors")
		policy.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	policy.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "AddressGroupBindingPolicy passed validation")

	// Проверяем что AddressGroup РЕАЛЬНО существует в committed состоянии
	_, err = reader.GetAddressGroupByID(ctx, policy.AddressGroupRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("AddressGroup %s not found", policy.AddressGroupRef.Key()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required AddressGroup not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get AddressGroup %s: %v", policy.AddressGroupRef.Key(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "AddressGroup validation failed")
		return nil
	}

	// Проверяем что Service РЕАЛЬНО существует в committed состоянии
	_, err = reader.GetServiceByID(ctx, policy.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Service %s not found", policy.ServiceRef.Key()))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required Service not found")
		return nil
	} else if err != nil {
		policy.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get Service %s: %v", policy.ServiceRef.Key(), err))
		policy.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Service validation failed")
		return nil
	}

	// Все проверки пройдены - политика готова
	policy.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "AddressGroupBindingPolicy is ready and operational")

	klog.V(4).Infof("ConditionManager.ProcessAddressGroupBindingPolicyConditions: policy %s/%s processed successfully", policy.Namespace, policy.Name)
	return nil
}

// ProcessIEAgAgRuleConditions формирует условия для IEAgAgRule ПОСЛЕ успешного commit
func (cm *ConditionManager) ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error {
	// Очищаем старые ошибки и обновляем метаданные
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	klog.V(4).Infof("ConditionManager.ProcessIEAgAgRuleConditions: processing rule %s/%s after commit", rule.Namespace, rule.Name)

	// Получаем reader для валидации
	reader, err := cm.registry.Reader(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return nil
	}
	defer reader.Close()

	// Backend синхронизирован (коммит прошел успешно)
	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "IEAgAgRule committed to backend successfully")

	// Создаем валидатор и выполняем валидацию РЕАЛЬНОГО состояния
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Проверяем базовую валидацию коммиченного объекта
	if err := ruleValidator.ValidateForCreation(ctx, *rule); err != nil {
		rule.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("IEAgAgRule validation failed: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "IEAgAgRule has validation errors")
		rule.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "IEAgAgRule passed validation")

	// Проверяем что AddressGroupLocal РЕАЛЬНО существует в committed состоянии
	_, err = reader.GetAddressGroupByID(ctx, rule.AddressGroupLocal.ResourceIdentifier)
	if err == ports.ErrNotFound {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Local AddressGroup %s not found", rule.AddressGroupLocal.Key()))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required local AddressGroup not found")
		return nil
	} else if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get local AddressGroup %s: %v", rule.AddressGroupLocal.Key(), err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Local AddressGroup validation failed")
		return nil
	}

	// Проверяем что AddressGroup РЕАЛЬНО существует в committed состоянии
	_, err = reader.GetAddressGroupByID(ctx, rule.AddressGroup.ResourceIdentifier)
	if err == ports.ErrNotFound {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Target AddressGroup %s not found", rule.AddressGroup.Key()))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Required target AddressGroup not found")
		return nil
	} else if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonDependencyError, fmt.Sprintf("Failed to get target AddressGroup %s: %v", rule.AddressGroup.Key(), err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Target AddressGroup validation failed")
		return nil
	}

	// Проверяем что у правила есть порты
	if len(rule.Ports) == 0 {
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonPending, "No ports configured")
		return nil
	}

	// Все проверки пройдены - правило готово
	portsCount := len(rule.Ports)
	rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("IEAgAgRule is ready, %d ports configured", portsCount))

	klog.V(4).Infof("ConditionManager.ProcessIEAgAgRuleConditions: rule %s/%s processed successfully", rule.Namespace, rule.Name)
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

	// Проверяем базовую валидацию коммиченного объекта
	klog.Infof("🔄 ConditionManager: Validating committed network %s/%s", network.Namespace, network.Name)
	if err := networkValidator.ValidateForCreation(ctx, *network); err != nil {
		klog.Errorf("❌ ConditionManager: Network validation failed for %s/%s: %v", network.Namespace, network.Name, err)
		network.Meta.SetErrorCondition(models.ReasonValidationFailed, fmt.Sprintf("Network validation failed: %v", err))
		network.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Network has validation errors")
		network.Meta.SetValidatedCondition(metav1.ConditionFalse, models.ReasonValidationFailed, fmt.Sprintf("Validation failed: %v", err))
		return nil
	}

	// Устанавливаем Validated = true
	klog.Infof("✅ ConditionManager: Setting Validated=true for %s/%s", network.Namespace, network.Name)
	network.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "Network passed validation")

	// Все проверки пройдены - сеть готова
	klog.Infof("🎉 ConditionManager: All checks passed, setting Ready=true for %s/%s", network.Namespace, network.Name)
	network.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Network is ready for use")

	klog.Infof("✅ ConditionManager.ProcessNetworkConditions: network %s/%s processed successfully with %d conditions", network.Namespace, network.Name, len(network.Meta.Conditions))
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
func (cm *ConditionManager) validateServiceAliasReferences(ctx context.Context, reader ports.Reader, rule *models.RuleS2S) error {
	// Проверяем локальный ServiceAlias
	localAlias, err := reader.GetServiceAliasByID(ctx, rule.ServiceLocalRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		return fmt.Errorf("local service alias '%s' not found", rule.ServiceLocalRef.Key())
	} else if err != nil {
		return fmt.Errorf("failed to get local service alias '%s': %v", rule.ServiceLocalRef.Key(), err)
	}

	// Проверяем целевой ServiceAlias
	targetAlias, err := reader.GetServiceAliasByID(ctx, rule.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		return fmt.Errorf("target service alias '%s' not found", rule.ServiceRef.Key())
	} else if err != nil {
		return fmt.Errorf("failed to get target service alias '%s': %v", rule.ServiceRef.Key(), err)
	}

	// Проверяем что Service, на которые ссылаются ServiceAlias, РЕАЛЬНО существуют
	_, err = reader.GetServiceByID(ctx, localAlias.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		return fmt.Errorf("local service '%s' (referenced by ServiceAlias '%s') not found", localAlias.ServiceRef.Key(), localAlias.Key())
	} else if err != nil {
		return fmt.Errorf("failed to get local service '%s': %v", localAlias.ServiceRef.Key(), err)
	}

	_, err = reader.GetServiceByID(ctx, targetAlias.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		return fmt.Errorf("target service '%s' (referenced by ServiceAlias '%s') not found", targetAlias.ServiceRef.Key(), targetAlias.Key())
	} else if err != nil {
		return fmt.Errorf("failed to get target service '%s': %v", targetAlias.ServiceRef.Key(), err)
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
		if err = writer.SyncServices(ctx, []models.Service{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.AddressGroup:
		if err = writer.SyncAddressGroups(ctx, []models.AddressGroup{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.RuleS2S:
		if err = writer.SyncRuleS2S(ctx, []models.RuleS2S{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.AddressGroupBinding:
		if err = writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.AddressGroupPortMapping:
		if err = writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.ServiceAlias:
		if err = writer.SyncServiceAliases(ctx, []models.ServiceAlias{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.AddressGroupBindingPolicy:
		if err = writer.SyncAddressGroupBindingPolicies(ctx, []models.AddressGroupBindingPolicy{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.IEAgAgRule:
		if err = writer.SyncIEAgAgRules(ctx, []models.IEAgAgRule{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.Network:
		if err = writer.SyncNetworks(ctx, []models.Network{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return err
		}
	case *models.NetworkBinding:
		if err = writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*r}, ports.NewResourceIdentifierScope(r.ResourceIdentifier), ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
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
