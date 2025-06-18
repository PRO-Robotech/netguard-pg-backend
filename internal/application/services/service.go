package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// NetguardService provides operations for managing netguard resources
type NetguardService struct {
	registry ports.Registry
}

// NewNetguardService creates a new NetguardService
func NewNetguardService(registry ports.Registry) *NetguardService {
	return &NetguardService{
		registry: registry,
	}
}

// GetServices returns all services
func (s *NetguardService) GetServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var services []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		services = append(services, service)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}
	return services, nil
}

// GetAddressGroups returns all address groups
func (s *NetguardService) GetAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var addressGroups []models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
		addressGroups = append(addressGroups, addressGroup)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address groups")
	}
	return addressGroups, nil
}

// GetAddressGroupBindings returns all address group bindings
func (s *NetguardService) GetAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var bindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		bindings = append(bindings, binding)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group bindings")
	}
	return bindings, nil
}

// GetAddressGroupPortMappings returns all address group port mappings
func (s *NetguardService) GetAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var mappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		mappings = append(mappings, mapping)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group port mappings")
	}
	return mappings, nil
}

// GetRuleS2S returns all rule s2s
func (s *NetguardService) GetRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list rule s2s")
	}
	return rules, nil
}

// GetServiceAliases returns all service aliases
func (s *NetguardService) GetServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var aliases []models.ServiceAlias
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		aliases = append(aliases, alias)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list service aliases")
	}
	return aliases, nil
}

// GetAddressGroupBindingPolicies returns all address group binding policies
func (s *NetguardService) GetAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var policies []models.AddressGroupBindingPolicy
	err = reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
		policies = append(policies, policy)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group binding policies")
	}
	return policies, nil
}

// CreateService создает новый сервис
func (s *NetguardService) CreateService(ctx context.Context, service models.Service) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Создаем валидатор
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Валидируем сервис перед созданием
	if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncServices(ctx, []models.Service{service}, ports.NewResourceIdentifierScope(service.ResourceIdentifier)); err != nil {
		return errors.Wrap(err, "failed to create service")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// UpdateService обновляет существующий сервис
func (s *NetguardService) UpdateService(ctx context.Context, service models.Service) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Получаем старую версию сервиса
	oldService, err := reader.GetServiceByID(ctx, service.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing service")
	}

	// Создаем валидатор
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Валидируем сервис перед обновлением
	if err := serviceValidator.ValidateForUpdate(ctx, *oldService, service); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncServices(ctx, []models.Service{service}, ports.NewResourceIdentifierScope(service.ResourceIdentifier)); err != nil {
		return errors.Wrap(err, "failed to update service")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// Sync выполняет синхронизацию с указанной операцией и субъектом
func (s *NetguardService) Sync(ctx context.Context, syncOp models.SyncOp, subject interface{}) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Обработка разных типов субъектов
	switch v := subject.(type) {
	case []models.Service:
		return s.syncServices(ctx, writer, v, syncOp)
	case []models.AddressGroup:
		return s.syncAddressGroups(ctx, writer, v, syncOp)
	case []models.AddressGroupBinding:
		return s.syncAddressGroupBindings(ctx, writer, v, syncOp)
	case []models.AddressGroupPortMapping:
		return s.syncAddressGroupPortMappings(ctx, writer, v, syncOp)
	case []models.RuleS2S:
		return s.syncRuleS2S(ctx, writer, v, syncOp)
	case []models.ServiceAlias:
		return s.syncServiceAliases(ctx, writer, v, syncOp)
	case []models.AddressGroupBindingPolicy:
		return s.syncAddressGroupBindingPolicies(ctx, writer, v, syncOp)
	default:
		return errors.New("unsupported subject type")
	}
}

// syncServices синхронизирует сервисы с указанной операцией
func (s *NetguardService) syncServices(ctx context.Context, writer ports.Writer, services []models.Service, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		serviceValidator := validator.GetServiceValidator()

		for _, service := range services {
			existingService, err := reader.GetServiceByID(ctx, service.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Сервис существует - используем ValidateForUpdate
				if err := serviceValidator.ValidateForUpdate(ctx, *existingService, service); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Сервис новый - используем ValidateForCreation
				if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get service")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if syncOp == models.SyncOpFullSync {
		// При операции FullSync используем пустую область видимости,
		// чтобы удалить все сервисы, а затем добавить только новые
		scope = ports.EmptyScope{}
	} else if len(services) > 0 {
		var ids []models.ResourceIdentifier
		for _, service := range services {
			ids = append(ids, service.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncServices(ctx, services, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync services")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// findRuleS2SForServices finds all RuleS2S that reference the given services
func (s *NetguardService) findRuleS2SForServices(ctx context.Context, serviceIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// First, find all ServiceAliases that reference these services
	var serviceAliases []models.ServiceAlias
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		for _, serviceID := range serviceIDs {
			if alias.ServiceRef.Key() == serviceID.Key() {
				serviceAliases = append(serviceAliases, alias)
				break
			}
		}
		return nil
	}, nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list service aliases")
	}

	// Create a map of service alias IDs for quick lookup
	serviceAliasMap := make(map[string]bool)
	for _, alias := range serviceAliases {
		serviceAliasMap[alias.Key()] = true
	}

	// Now find all RuleS2S that reference these service aliases
	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Check if the rule references any of the service aliases
		if serviceAliasMap[rule.ServiceLocalRef.Key()] || serviceAliasMap[rule.ServiceRef.Key()] {
			rules = append(rules, rule)
		}
		return nil
	}, nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list rules")
	}

	return rules, nil
}

// updateIEAgAgRulesForRuleS2S updates the IEAgAgRules for the given RuleS2S
// syncOp - операция синхронизации (FullSync, Upsert, Delete)
func (s *NetguardService) updateIEAgAgRulesForRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	// Get all existing IEAgAgRules to detect obsolete ones
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}

	existingRules := make(map[string]models.IEAgAgRule)
	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		existingRules[rule.Key()] = rule
		return nil
	}, nil)
	reader.Close()

	if err != nil {
		return errors.Wrap(err, "failed to list existing IEAgAgRules")
	}

	// Create a map of expected rules after the update
	expectedRules := make(map[string]bool)
	var allNewRules []models.IEAgAgRule

	// Generate IEAgAgRules for each RuleS2S
	for _, rule := range rules {
		ieAgAgRules, err := s.GenerateIEAgAgRulesFromRuleS2S(ctx, rule)
		if err != nil {
			return errors.Wrapf(err, "failed to generate IEAgAgRules for RuleS2S %s", rule.Key())
		}

		// Add generated rules to the expected rules map and collect all new rules
		for _, ieRule := range ieAgAgRules {
			expectedRules[ieRule.Key()] = true
			allNewRules = append(allNewRules, ieRule)
		}
	}

	// Sync all new rules at once
	if len(allNewRules) > 0 {
		if err = writer.SyncIEAgAgRules(ctx, allNewRules, nil, ports.WithSyncOp(syncOp)); err != nil {
			return errors.Wrap(err, "failed to sync new IEAgAgRules")
		}
	}

	// Find and delete obsolete rules
	var obsoleteRules []models.IEAgAgRule
	for key, rule := range existingRules {
		if !expectedRules[key] {
			obsoleteRules = append(obsoleteRules, rule)
		}
	}

	if len(obsoleteRules) > 0 {
		var obsoleteRuleIDs []models.ResourceIdentifier
		for _, rule := range obsoleteRules {
			obsoleteRuleIDs = append(obsoleteRuleIDs, rule.ResourceIdentifier)
		}

		if err = writer.DeleteIEAgAgRulesByIDs(ctx, obsoleteRuleIDs); err != nil {
			return errors.Wrap(err, "failed to delete obsolete IEAgAgRules")
		}
	}

	return nil
}

// SyncServices syncs services
func (s *NetguardService) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Validate all services
	for _, service := range services {
		// Check if service exists
		existingService, err := reader.GetServiceByID(ctx, service.ResourceIdentifier)
		if err == nil {
			// Service exists - use ValidateForUpdate
			if err := serviceValidator.ValidateForUpdate(ctx, *existingService, service); err != nil {
				return err
			}
		} else {
			// Service is new - use ValidateForCreation
			if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
				return err
			}
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncServices(ctx, services, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync services")
	}

	// After successfully syncing services, update related IEAgAgRules
	// Collect service IDs
	var serviceIDs []models.ResourceIdentifier
	for _, service := range services {
		serviceIDs = append(serviceIDs, service.ResourceIdentifier)
	}

	// Find all RuleS2S that reference these services
	affectedRules, err := s.findRuleS2SForServices(ctx, serviceIDs)
	if err != nil {
		writer.Abort()
		return errors.Wrap(err, "failed to find affected RuleS2S")
	}

	// Update IEAgAgRules for affected RuleS2S
	if len(affectedRules) > 0 {
		if err = s.updateIEAgAgRulesForRuleS2S(ctx, writer, affectedRules, models.SyncOpFullSync); err != nil {
			writer.Abort()
			return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncAddressGroups синхронизирует группы адресов с указанной операцией
func (s *NetguardService) syncAddressGroups(ctx context.Context, writer ports.Writer, addressGroups []models.AddressGroup, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		addressGroupValidator := validator.GetAddressGroupValidator()

		for _, addressGroup := range addressGroups {
			existingAddressGroup, err := reader.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Группа адресов существует - используем ValidateForUpdate
				if err := addressGroupValidator.ValidateForUpdate(ctx, *existingAddressGroup, addressGroup); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Группа адресов новая - используем ValidateForCreation
				if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get address group")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if syncOp == models.SyncOpFullSync {
		// При операции FullSync используем пустую область видимости,
		// чтобы удалить все группы адресов, а затем добавить только новые
		scope = ports.EmptyScope{}
	} else if len(addressGroups) > 0 {
		var ids []models.ResourceIdentifier
		for _, addressGroup := range addressGroups {
			ids = append(ids, addressGroup.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncAddressGroups(ctx, addressGroups, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// findServicesForAddressGroups finds all Services that reference the given address groups
func (s *NetguardService) findServicesForAddressGroups(ctx context.Context, addressGroupIDs []models.ResourceIdentifier) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create a map of address group IDs for quick lookup
	addressGroupMap := make(map[string]bool)
	for _, id := range addressGroupIDs {
		addressGroupMap[id.Key()] = true
	}

	// Find all Services that reference these address groups
	var services []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		// Check if the service references any of the address groups
		for _, ag := range service.AddressGroups {
			if addressGroupMap[ag.Key()] {
				services = append(services, service)
				break
			}
		}
		return nil
	}, nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}

	return services, nil
}

// SyncAddressGroups syncs address groups
func (s *NetguardService) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Validate all address groups
	for _, addressGroup := range addressGroups {
		// Check if address group exists
		existingAddressGroup, err := reader.GetAddressGroupByID(ctx, addressGroup.ResourceIdentifier)
		if err == nil {
			// Address group exists - use ValidateForUpdate
			if err := addressGroupValidator.ValidateForUpdate(ctx, *existingAddressGroup, addressGroup); err != nil {
				return err
			}
		} else if err == ports.ErrNotFound {
			// Address group is new - use ValidateForCreation
			if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
				return err
			}
		} else {
			// Other error occurred
			return errors.Wrap(err, "failed to get address group")
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncAddressGroups(ctx, addressGroups, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}

	// After successfully syncing address groups, update related IEAgAgRules
	// Collect address group IDs
	var addressGroupIDs []models.ResourceIdentifier
	for _, ag := range addressGroups {
		addressGroupIDs = append(addressGroupIDs, ag.ResourceIdentifier)
	}

	// Find all Services that reference these address groups
	affectedServices, err := s.findServicesForAddressGroups(ctx, addressGroupIDs)
	if err != nil {
		writer.Abort()
		return errors.Wrap(err, "failed to find affected Services")
	}

	// Collect service IDs
	var serviceIDs []models.ResourceIdentifier
	for _, service := range affectedServices {
		serviceIDs = append(serviceIDs, service.ResourceIdentifier)
	}

	// Find all RuleS2S that reference these services
	affectedRules, err := s.findRuleS2SForServices(ctx, serviceIDs)
	if err != nil {
		writer.Abort()
		return errors.Wrap(err, "failed to find affected RuleS2S")
	}

	// Update IEAgAgRules for affected RuleS2S
	if len(affectedRules) > 0 {
		if err = s.updateIEAgAgRulesForRuleS2S(ctx, writer, affectedRules, models.SyncOpFullSync); err != nil {
			writer.Abort()
			return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncAddressGroupBindings синхронизирует привязки групп адресов с указанной операцией
func (s *NetguardService) syncAddressGroupBindings(ctx context.Context, writer ports.Writer, bindings []models.AddressGroupBinding, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		bindingValidator := validator.GetAddressGroupBindingValidator()

		for i := range bindings {
			// Use pointer to binding so we can modify it
			binding := &bindings[i]

			existingBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Привязка существует - используем ValidateForUpdate
				if err := bindingValidator.ValidateForUpdate(ctx, *existingBinding, binding); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Привязка новая - используем ValidateForCreation
				if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get address group binding")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if syncOp == models.SyncOpFullSync {
		// При операции FullSync используем пустую область видимости,
		// чтобы удалить все привязки групп адресов, а затем добавить только новые
		scope = ports.EmptyScope{}
	} else if len(bindings) > 0 {
		var ids []models.ResourceIdentifier
		for _, binding := range bindings {
			ids = append(ids, binding.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncAddressGroupBindings(ctx, bindings, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings")
	}

	// Синхронизируем port mappings для каждого binding, если это не удаление
	if syncOp != models.SyncOpDelete {
		for _, binding := range bindings {
			// Игнорируем ошибки при синхронизации port mappings, чтобы не блокировать основную операцию
			_ = s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, syncOp)
		}
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// SyncAddressGroupPortMappingsWithWriter обеспечивает синхронизацию port mapping для binding
// writer - существующий открытый writer для транзакции
// syncOp - операция синхронизации (FullSync, Upsert, Delete)
func (s *NetguardService) SyncAddressGroupPortMappingsWithWriter(ctx context.Context, writer ports.Writer, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Получаем сервис для доступа к его портам
	service, err := reader.GetServiceByID(ctx, binding.ServiceRef.ResourceIdentifier)
	if err == ports.ErrNotFound {
		return errors.New("service not found for port mapping")
	} else if err != nil {
		return errors.Wrapf(err, "failed to get service for port mapping")
	}

	// Проверяем существующий port mapping для этой address group
	portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, binding.AddressGroupRef.ResourceIdentifier)

	var updatedMapping models.AddressGroupPortMapping

	if err == ports.ErrNotFound {
		// Port mapping не существует - создаем новый
		updatedMapping = *validation.CreateNewPortMapping(binding.AddressGroupRef.ResourceIdentifier, *service)
	} else if err != nil {
		// Произошла другая ошибка
		return errors.Wrap(err, "failed to get address group port mapping")
	} else {
		// Port mapping существует - обновляем его
		updatedMapping = *validation.UpdatePortMapping(*portMapping, binding.ServiceRef, *service)

		// Проверяем перекрытие портов
		if err := validation.CheckPortOverlaps(*service, updatedMapping); err != nil {
			return err
		}
	}

	// Используем переданный writer вместо создания нового
	if err = writer.SyncAddressGroupPortMappings(
		ctx,
		[]models.AddressGroupPortMapping{updatedMapping},
		ports.NewResourceIdentifierScope(updatedMapping.ResourceIdentifier),
		ports.WithSyncOp(syncOp),
	); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}

	return nil
}

// SyncAddressGroupPortMappings обеспечивает синхронизацию port mapping для binding
// с созданием собственной транзакции, используя операцию Upsert
func (s *NetguardService) SyncAddressGroupPortMappings(ctx context.Context, binding models.AddressGroupBinding) error {
	return s.SyncAddressGroupPortMappingsWithSyncOp(ctx, binding, models.SyncOpUpsert)
}

// SyncAddressGroupPortMappingsWithSyncOp обеспечивает синхронизацию port mapping для binding
// с созданием собственной транзакции и указанной операцией синхронизации
func (s *NetguardService) SyncAddressGroupPortMappingsWithSyncOp(ctx context.Context, binding models.AddressGroupBinding, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.SyncAddressGroupPortMappingsWithWriter(ctx, writer, binding, syncOp); err != nil {
		return err
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// SyncAddressGroupBindings syncs address group bindings
func (s *NetguardService) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Validate all bindings
	for i := range bindings {
		binding := &bindings[i]

		// Check if binding exists
		existingBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.ResourceIdentifier)
		if err == nil {
			// Binding exists - use ValidateForUpdate
			if err := bindingValidator.ValidateForUpdate(ctx, *existingBinding, binding); err != nil {
				return err
			}
		} else {
			// Binding is new - use ValidateForCreation
			if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
				return err
			}
		}
	}

	// Создаем единую транзакцию для всех операций
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Sync bindings
	if err = writer.SyncAddressGroupBindings(ctx, bindings, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings")
	}

	// Синхронизируем port mappings для каждого binding в той же транзакции
	for _, binding := range bindings {
		if err := s.SyncAddressGroupPortMappingsWithWriter(ctx, writer, binding, models.SyncOpFullSync); err != nil {
			return errors.Wrapf(err, "failed to sync port mapping for binding %s", binding.Key())
		}
	}

	// Фиксируем все изменения в одной транзакции
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncAddressGroupPortMappings синхронизирует маппинги портов групп адресов с указанной операцией
// Не вызывает Commit() - это должен делать вызывающий метод
func (s *NetguardService) syncAddressGroupPortMappings(ctx context.Context, writer ports.Writer, mappings []models.AddressGroupPortMapping, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		mappingValidator := validator.GetAddressGroupPortMappingValidator()

		for _, mapping := range mappings {
			existingMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Маппинг существует - используем ValidateForUpdate
				if err := mappingValidator.ValidateForUpdate(ctx, *existingMapping, mapping); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Маппинг новый - используем ValidateForCreation
				if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get address group port mapping")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if len(mappings) > 0 {
		var ids []models.ResourceIdentifier
		for _, mapping := range mappings {
			ids = append(ids, mapping.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncAddressGroupPortMappings(ctx, mappings, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}

	// Не вызываем Commit() - это должен делать вызывающий метод

	return nil
}

// SyncMultipleAddressGroupPortMappings syncs multiple address group port mappings
func (s *NetguardService) SyncMultipleAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Validate all mappings
	for _, mapping := range mappings {
		// Check if mapping exists
		existingMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.ResourceIdentifier)
		if err == nil {
			// Mapping exists - use ValidateForUpdate
			if err := mappingValidator.ValidateForUpdate(ctx, *existingMapping, mapping); err != nil {
				return err
			}
		} else if err == ports.ErrNotFound {
			// Mapping is new - use ValidateForCreation
			if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
				return err
			}
		} else {
			// Other error occurred
			return errors.Wrap(err, "failed to get address group port mapping")
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Используем обновленный метод syncAddressGroupPortMappings
	if err = s.syncAddressGroupPortMappings(ctx, writer, mappings, models.SyncOpFullSync); err != nil {
		return err
	}

	// Фиксируем транзакцию
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncRuleS2S синхронизирует правила s2s с указанной операцией
func (s *NetguardService) syncRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		ruleValidator := validator.GetRuleS2SValidator()

		for _, rule := range rules {
			existingRule, err := reader.GetRuleS2SByID(ctx, rule.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Правило существует - используем ValidateForUpdate
				if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Правило новое - используем ValidateForCreation
				if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get rule s2s")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if len(rules) > 0 {
		var ids []models.ResourceIdentifier
		for _, rule := range rules {
			ids = append(ids, rule.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncRuleS2S(ctx, rules, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync rule s2s")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// SyncRuleS2S syncs rule s2s
func (s *NetguardService) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Validate all rules
	for _, rule := range rules {
		// Check if rule exists
		existingRule, err := reader.GetRuleS2SByID(ctx, rule.ResourceIdentifier)
		if err == nil {
			// Rule exists - use ValidateForUpdate
			if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
				return err
			}
		} else if err == ports.ErrNotFound {
			// Rule is new - use ValidateForCreation
			if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
				return err
			}
		} else {
			// Other error occurred
			return errors.Wrap(err, "failed to get rule s2s")
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncRuleS2S(ctx, rules, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync rule s2s")
	}

	// Get all existing IEAgAgRules to detect obsolete ones
	existingRules, err := s.GetIEAgAgRules(ctx, nil)
	if err != nil {
		writer.Abort()
		return errors.Wrap(err, "failed to get existing IEAgAgRules")
	}

	// Create a map of expected rules after the sync operation
	expectedRules := make(map[string]bool)
	var allNewRules []models.IEAgAgRule

	// After successfully syncing RuleS2S, update related IEAgAgRules
	for _, rule := range rules {
		ieAgAgRules, err := s.GenerateIEAgAgRulesFromRuleS2S(ctx, rule)
		if err != nil {
			writer.Abort()
			return errors.Wrapf(err, "failed to generate IEAgAgRules for RuleS2S %s", rule.Key())
		}

		// Add generated rules to the expected rules map and collect all new rules
		for _, ieRule := range ieAgAgRules {
			expectedRules[ieRule.Key()] = true
			allNewRules = append(allNewRules, ieRule)
		}
	}

	// Sync all new rules at once
	if len(allNewRules) > 0 {
		if err = writer.SyncIEAgAgRules(ctx, allNewRules, nil, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
			writer.Abort()
			return errors.Wrap(err, "failed to sync new IEAgAgRules")
		}
	}

	// Find and delete obsolete rules
	var obsoleteRuleIDs []models.ResourceIdentifier
	for _, existingRule := range existingRules {
		if !expectedRules[existingRule.Key()] {
			obsoleteRuleIDs = append(obsoleteRuleIDs, existingRule.ResourceIdentifier)
		}
	}

	if len(obsoleteRuleIDs) > 0 {
		if err = writer.DeleteIEAgAgRulesByIDs(ctx, obsoleteRuleIDs); err != nil {
			writer.Abort()
			return errors.Wrap(err, "failed to delete obsolete IEAgAgRules")
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncServiceAliases синхронизирует алиасы сервисов с указанной операцией
func (s *NetguardService) syncServiceAliases(ctx context.Context, writer ports.Writer, aliases []models.ServiceAlias, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		aliasValidator := validator.GetServiceAliasValidator()

		for i := range aliases {
			// Используем указатель на элемент слайса, чтобы изменения сохранились
			alias := &aliases[i]

			existingAlias, err := reader.GetServiceAliasByID(ctx, alias.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Алиас существует - используем ValidateForUpdate
				if err := aliasValidator.ValidateForUpdate(ctx, *existingAlias, *alias); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Алиас новый - используем ValidateForCreation
				if err := aliasValidator.ValidateForCreation(ctx, alias); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get service alias")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if len(aliases) > 0 {
		var ids []models.ResourceIdentifier
		for _, alias := range aliases {
			ids = append(ids, alias.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncServiceAliases(ctx, aliases, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync service aliases")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// findRuleS2SForServiceAliases finds all RuleS2S that reference the given service aliases
func (s *NetguardService) findRuleS2SForServiceAliases(ctx context.Context, aliasIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create a map of service alias IDs for quick lookup
	aliasMap := make(map[string]bool)
	for _, id := range aliasIDs {
		aliasMap[id.Key()] = true
	}

	// Find all RuleS2S that reference these service aliases
	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Check if the rule references any of the service aliases
		if aliasMap[rule.ServiceLocalRef.Key()] || aliasMap[rule.ServiceRef.Key()] {
			rules = append(rules, rule)
		}
		return nil
	}, nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list rules")
	}

	return rules, nil
}

// SyncServiceAliases syncs service aliases
func (s *NetguardService) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Validate all aliases
	for i := range aliases {
		// Используем указатель на элемент слайса, чтобы изменения сохранились
		alias := &aliases[i]

		// Check if alias exists
		existingAlias, err := reader.GetServiceAliasByID(ctx, alias.ResourceIdentifier)
		if err == nil {
			// Alias exists - use ValidateForUpdate
			if err := aliasValidator.ValidateForUpdate(ctx, *existingAlias, *alias); err != nil {
				return err
			}
		} else if err == ports.ErrNotFound {
			// Alias is new - use ValidateForCreation
			if err := aliasValidator.ValidateForCreation(ctx, alias); err != nil {
				return err
			}
		} else {
			// Other error occurred
			return errors.Wrap(err, "failed to get service alias")
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncServiceAliases(ctx, aliases, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync service aliases")
	}

	// After successfully syncing service aliases, update related IEAgAgRules
	// Collect service alias IDs
	var aliasIDs []models.ResourceIdentifier
	for _, alias := range aliases {
		aliasIDs = append(aliasIDs, alias.ResourceIdentifier)
	}

	// Find all RuleS2S that reference these service aliases
	affectedRules, err := s.findRuleS2SForServiceAliases(ctx, aliasIDs)
	if err != nil {
		writer.Abort()
		return errors.Wrap(err, "failed to find affected RuleS2S")
	}

	// Update IEAgAgRules for affected RuleS2S
	if len(affectedRules) > 0 {
		if err = s.updateIEAgAgRulesForRuleS2S(ctx, writer, affectedRules, models.SyncOpFullSync); err != nil {
			writer.Abort()
			return errors.Wrap(err, "failed to update IEAgAgRules for affected RuleS2S")
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// GetSyncStatus returns the status of the last synchronization
func (s *NetguardService) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetSyncStatus(ctx)
}

// GetServiceByID returns a service by ID
func (s *NetguardService) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	service, err := reader.GetServiceByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service")
	}

	return service, nil
}

// GetAddressGroupByID returns an address group by ID
func (s *NetguardService) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group")
	}

	return addressGroup, nil
}

// GetAddressGroupBindingByID returns an address group binding by ID
func (s *NetguardService) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	binding, err := reader.GetAddressGroupBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding")
	}

	return binding, nil
}

// GetAddressGroupPortMappingByID returns an address group port mapping by ID
func (s *NetguardService) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	mapping, err := reader.GetAddressGroupPortMappingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group port mapping")
	}

	return mapping, nil
}

// GetRuleS2SByID returns a rule s2s by ID
func (s *NetguardService) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	rule, err := reader.GetRuleS2SByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get rule s2s")
	}

	return rule, nil
}

// GetServiceAliasByID returns a service alias by ID
func (s *NetguardService) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	alias, err := reader.GetServiceAliasByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service alias")
	}

	return alias, nil
}

// GetAddressGroupBindingPolicyByID returns an address group binding policy by ID
func (s *NetguardService) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	policy, err := reader.GetAddressGroupBindingPolicyByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding policy")
	}

	return policy, nil
}

// GetServicesByIDs returns services by IDs
func (s *NetguardService) GetServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var services []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		services = append(services, service)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}

	return services, nil
}

// GetAddressGroupsByIDs returns address groups by IDs
func (s *NetguardService) GetAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var addressGroups []models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
		addressGroups = append(addressGroups, addressGroup)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list address groups")
	}

	return addressGroups, nil
}

// GetAddressGroupBindingsByIDs returns address group bindings by IDs
func (s *NetguardService) GetAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var bindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		bindings = append(bindings, binding)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group bindings")
	}

	return bindings, nil
}

// GetAddressGroupPortMappingsByIDs returns address group port mappings by IDs
func (s *NetguardService) GetAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var mappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		mappings = append(mappings, mapping)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group port mappings")
	}

	return mappings, nil
}

// GetRuleS2SByIDs returns rules s2s by IDs
func (s *NetguardService) GetRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		rules = append(rules, rule)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list rules s2s")
	}

	return rules, nil
}

// GetServiceAliasesByIDs returns service aliases by IDs
func (s *NetguardService) GetServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var aliases []models.ServiceAlias
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		aliases = append(aliases, alias)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list service aliases")
	}

	return aliases, nil
}

// GetAddressGroupBindingPoliciesByIDs returns address group binding policies by IDs
func (s *NetguardService) GetAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.AddressGroupBindingPolicy, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var policies []models.AddressGroupBindingPolicy
	err = reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
		policies = append(policies, policy)
		return nil
	}, ports.NewResourceIdentifierScope(ids...))

	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group binding policies")
	}

	return policies, nil
}

// DeleteServicesByIDs deletes services by IDs
func (s *NetguardService) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Check dependencies for each service
	for _, id := range ids {
		if err := serviceValidator.CheckDependencies(ctx, id); err != nil {
			return err
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteServicesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete services")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteAddressGroupsByIDs deletes address groups by IDs
func (s *NetguardService) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Check dependencies for each address group
	for _, id := range ids {
		if err := addressGroupValidator.CheckDependencies(ctx, id); err != nil {
			return err
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address groups")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteAddressGroupBindingsByIDs deletes address group bindings by IDs
func (s *NetguardService) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Получаем bindings, которые будут удалены
	var bindings []models.AddressGroupBinding
	for _, id := range ids {
		binding, err := reader.GetAddressGroupBindingByID(ctx, id)
		if err != nil {
			continue // Пропускаем, если binding не существует
		}
		bindings = append(bindings, *binding)
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Удаляем bindings
	if err = writer.DeleteAddressGroupBindingsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings")
	}

	// Обновляем port mappings для каждого удаленного binding
	for _, binding := range bindings {
		// Проверяем, есть ли другие bindings для той же address group
		hasOtherBindings := false
		err = reader.ListAddressGroupBindings(ctx, func(b models.AddressGroupBinding) error {
			if b.AddressGroupRef.Key() == binding.AddressGroupRef.Key() && b.Key() != binding.Key() {
				hasOtherBindings = true
			}
			return nil
		}, nil)

		if err != nil {
			return errors.Wrap(err, "failed to check for other bindings")
		}

		// Если нет других bindings, удаляем port mapping
		if !hasOtherBindings {
			if err = writer.DeleteAddressGroupPortMappingsByIDs(ctx, []models.ResourceIdentifier{binding.AddressGroupRef.ResourceIdentifier}); err != nil {
				return errors.Wrap(err, "failed to delete address group port mappings")
			}
		} else {
			// Иначе обновляем port mapping, удаляя сервис
			portMapping, err := reader.GetAddressGroupPortMappingByID(ctx, binding.AddressGroupRef.ResourceIdentifier)
			if err != nil {
				continue // Пропускаем, если port mapping не существует
			}

			// Удаляем сервис из port mapping
			delete(portMapping.AccessPorts, binding.ServiceRef)

			// Обновляем port mapping
			if err = writer.SyncAddressGroupPortMappings(
				ctx,
				[]models.AddressGroupPortMapping{*portMapping},
				ports.NewResourceIdentifierScope(portMapping.ResourceIdentifier),
				ports.WithSyncOp(models.SyncOpUpsert),
			); err != nil {
				return errors.Wrap(err, "failed to update address group port mappings")
			}
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteAddressGroupPortMappingsByIDs deletes address group port mappings by IDs
func (s *NetguardService) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// Note: Address group port mappings don't have dependencies, so we don't need to check for them
	// However, we could add validation to ensure the mappings exist before deleting them

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupPortMappingsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteRuleS2SByIDs deletes rules s2s by IDs
func (s *NetguardService) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// Note: Rules don't have dependencies, so we don't need to check for them
	// However, we could add validation to ensure the rules exist before deleting them

	// First, get the RuleS2S objects to generate IEAgAgRules for deletion
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}

	// Collect all IEAgAgRule IDs that need to be deleted
	var ieAgAgRuleIDs []models.ResourceIdentifier

	// For each RuleS2S, generate and collect IEAgAgRules for deletion
	for _, id := range ids {
		ruleS2S, err := reader.GetRuleS2SByID(ctx, id)
		if err != nil || ruleS2S == nil {
			// Skip if rule not found
			continue
		}

		// Generate IEAgAgRules for this RuleS2S
		ieAgAgRules, err := s.GenerateIEAgAgRulesFromRuleS2S(ctx, *ruleS2S)
		if err != nil {
			reader.Close()
			return errors.Wrapf(err, "failed to generate IEAgAgRules for RuleS2S %s", id.Key())
		}

		// Collect IDs of generated IEAgAgRules
		for _, rule := range ieAgAgRules {
			ieAgAgRuleIDs = append(ieAgAgRuleIDs, rule.ResourceIdentifier)
		}
	}

	reader.Close()

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// First delete the associated IEAgAgRules if any were found
	if len(ieAgAgRuleIDs) > 0 {
		if err = writer.DeleteIEAgAgRulesByIDs(ctx, ieAgAgRuleIDs); err != nil {
			return errors.Wrap(err, "failed to delete associated IEAgAgRules")
		}
	}

	// Then delete the RuleS2S objects
	if err = writer.DeleteRuleS2SByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete rules s2s")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteServiceAliasesByIDs deletes service aliases by IDs
func (s *NetguardService) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Check dependencies for each service alias
	for _, id := range ids {
		if err := aliasValidator.CheckDependencies(ctx, id); err != nil {
			return err
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteServiceAliasesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete service aliases")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// syncAddressGroupBindingPolicies синхронизирует политики привязки групп адресов с указанной операцией
func (s *NetguardService) syncAddressGroupBindingPolicies(ctx context.Context, writer ports.Writer, policies []models.AddressGroupBindingPolicy, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		policyValidator := validator.GetAddressGroupBindingPolicyValidator()

		for i := range policies {
			// Используем указатель на элемент слайса, чтобы изменения сохранились
			policy := &policies[i]

			existingPolicy, err := reader.GetAddressGroupBindingPolicyByID(ctx, policy.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Политика существует - используем ValidateForUpdate
				if err := policyValidator.ValidateForUpdate(ctx, *existingPolicy, policy); err != nil {
					return err
				}
			} else if err == ports.ErrNotFound && syncOp != models.SyncOpDelete {
				// Политика новая - используем ValidateForCreation
				if err := policyValidator.ValidateForCreation(ctx, policy); err != nil {
					return err
				}
			} else if err != nil && err != ports.ErrNotFound {
				// Произошла другая ошибка
				return errors.Wrap(err, "failed to get address group binding policy")
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if len(policies) > 0 {
		var ids []models.ResourceIdentifier
		for _, policy := range policies {
			ids = append(ids, policy.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncAddressGroupBindingPolicies(ctx, policies, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync address group binding policies")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// SyncAddressGroupBindingPolicies syncs address group binding policies
func (s *NetguardService) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	// Validate all policies
	for i := range policies {
		// Используем указатель на элемент слайса, чтобы изменения сохранились
		policy := &policies[i]

		// Check if policy exists
		existingPolicy, err := reader.GetAddressGroupBindingPolicyByID(ctx, policy.ResourceIdentifier)
		if err == nil {
			// Policy exists - use ValidateForUpdate
			if err := policyValidator.ValidateForUpdate(ctx, *existingPolicy, policy); err != nil {
				return err
			}
		} else if err == ports.ErrNotFound {
			// Policy is new - use ValidateForCreation
			if err := policyValidator.ValidateForCreation(ctx, policy); err != nil {
				return err
			}
		} else {
			// Other error occurred
			return errors.Wrap(err, "failed to get address group binding policy")
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncAddressGroupBindingPolicies(ctx, policies, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync address group binding policies")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// GetIEAgAgRules returns a list of IEAgAgRules
func (s *NetguardService) GetIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var result []models.IEAgAgRule

	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		result = append(result, rule)
		return nil
	}, scope)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list IEAgAgRules")
	}

	return result, nil
}

// GetIEAgAgRuleByID returns a IEAgAgRule by ID
func (s *NetguardService) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetIEAgAgRuleByID(ctx, id)
}

// syncIEAgAgRules синхронизирует правила IEAgAgRule с указанной операцией
func (s *NetguardService) syncIEAgAgRules(ctx context.Context, writer ports.Writer, rules []models.IEAgAgRule, syncOp models.SyncOp) error {
	// Валидация в зависимости от операции
	if syncOp != models.SyncOpDelete {
		reader, err := s.registry.Reader(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get reader")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		ruleValidator := validator.GetIEAgAgRuleValidator()

		for _, rule := range rules {
			existingRule, err := reader.GetIEAgAgRuleByID(ctx, rule.ResourceIdentifier)
			if err == nil && syncOp != models.SyncOpDelete {
				// Правило существует - используем ValidateForUpdate
				if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
					return err
				}
			} else if syncOp != models.SyncOpDelete {
				// Правило новое - используем ValidateForCreation
				if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
					return err
				}
			}
		}
	}

	// Определение scope
	var scope ports.Scope
	if len(rules) > 0 {
		var ids []models.ResourceIdentifier
		for _, rule := range rules {
			ids = append(ids, rule.ResourceIdentifier)
		}
		scope = ports.NewResourceIdentifierScope(ids...)
	} else {
		scope = ports.EmptyScope{}
	}

	// Выполнение операции с указанной опцией
	if err := writer.SyncIEAgAgRules(ctx, rules, scope, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAgRules")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// GenerateIEAgAgRulesFromRuleS2S генерирует правила IEAgAgRule на основе RuleS2S
func (s *NetguardService) GenerateIEAgAgRulesFromRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Получаем сервисы по ссылкам
	localServiceAlias, err := reader.GetServiceAliasByID(ctx, ruleS2S.ServiceLocalRef.ResourceIdentifier)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service alias %s", ruleS2S.ServiceLocalRef.Key())
	}

	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, ruleS2S.ServiceRef.ResourceIdentifier)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service alias %s", ruleS2S.ServiceRef.Key())
	}

	localService, err := reader.GetServiceByID(ctx, localServiceAlias.ServiceRef.ResourceIdentifier)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", localServiceAlias.ServiceRef.Key())
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceAlias.ServiceRef.ResourceIdentifier)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", targetServiceAlias.ServiceRef.Key())
	}

	// Получаем группы адресов
	localAddressGroups := localService.AddressGroups
	targetAddressGroups := targetService.AddressGroups

	// Определяем порты в зависимости от направления трафика
	var ports []models.IngressPort
	if ruleS2S.Traffic == models.INGRESS {
		ports = localService.IngressPorts
	} else {
		ports = targetService.IngressPorts
	}

	// Создаем правила IEAgAgRule
	var result []models.IEAgAgRule

	// Группируем порты по протоколу
	tcpPorts := []string{}
	udpPorts := []string{}

	for _, port := range ports {
		if port.Protocol == models.TCP {
			tcpPorts = append(tcpPorts, port.Port)
		} else if port.Protocol == models.UDP {
			udpPorts = append(udpPorts, port.Port)
		}
	}

	// Создаем правила для каждой комбинации групп адресов и протоколов
	for _, localAG := range localAddressGroups {
		for _, targetAG := range targetAddressGroups {
			// Создаем TCP правило
			if len(tcpPorts) > 0 {
				tcpRule := models.IEAgAgRule{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.NewResourceIdentifier(
							generateRuleName(string(ruleS2S.Traffic), localAG.Name, targetAG.Name, string(models.TCP)),
							models.WithNamespace(determineRuleNamespace(ruleS2S, localAG, targetAG)),
						),
					},
					Transport:         models.TCP,
					Traffic:           ruleS2S.Traffic,
					AddressGroupLocal: localAG,
					AddressGroup:      targetAG,
					Ports: []models.PortSpec{
						{
							Destination: strings.Join(tcpPorts, ","),
						},
					},
					Action:   models.ActionAccept,
					Logs:     true,
					Priority: 100,
				}
				result = append(result, tcpRule)
			}

			// Создаем UDP правило
			if len(udpPorts) > 0 {
				udpRule := models.IEAgAgRule{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.NewResourceIdentifier(
							generateRuleName(string(ruleS2S.Traffic), localAG.Name, targetAG.Name, string(models.UDP)),
							models.WithNamespace(determineRuleNamespace(ruleS2S, localAG, targetAG)),
						),
					},
					Transport:         models.UDP,
					Traffic:           ruleS2S.Traffic,
					AddressGroupLocal: localAG,
					AddressGroup:      targetAG,
					Ports: []models.PortSpec{
						{
							Destination: strings.Join(udpPorts, ","),
						},
					},
					Action:   models.ActionAccept,
					Logs:     true,
					Priority: 100,
				}
				result = append(result, udpRule)
			}
		}
	}

	return result, nil
}

// generateRuleName создает детерминированное имя правила
func generateRuleName(trafficDirection, localAGName, targetAGName, protocol string) string {
	input := fmt.Sprintf("%s-%s-%s-%s",
		strings.ToLower(trafficDirection),
		localAGName,
		targetAGName,
		strings.ToLower(protocol))

	h := sha256.New()
	h.Write([]byte(input))
	hash := h.Sum(nil)

	// Форматируем первые 16 байт как UUID v5
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])

	// Используем префикс направления трафика и UUID
	return fmt.Sprintf("%s-%s",
		strings.ToLower(trafficDirection)[:3],
		uuid)
}

// determineRuleNamespace определяет пространство имен для правила
func determineRuleNamespace(ruleS2S models.RuleS2S, localAG, targetAG models.AddressGroupRef) string {
	if ruleS2S.Traffic == models.INGRESS {
		// Для входящего трафика правило размещается в пространстве имен локальной группы адресов
		if localAG.Namespace != "" {
			return localAG.Namespace
		}
		return ruleS2S.Namespace
	} else {
		// Для исходящего трафика правило размещается в пространстве имен целевой группы адресов
		if targetAG.Namespace != "" {
			return targetAG.Namespace
		}
		return ruleS2S.Namespace
	}
}

// SyncIEAgAgRules синхронизирует правила IEAgAgRule
func (s *NetguardService) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Validate all rules
	for _, rule := range rules {
		// Check if rule exists
		existingRule, err := reader.GetIEAgAgRuleByID(ctx, rule.ResourceIdentifier)
		if err == nil {
			// Rule exists - use ValidateForUpdate
			if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
				return err
			}
		} else {
			// Rule is new - use ValidateForCreation
			if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
				return err
			}
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncIEAgAgRules(ctx, rules, scope, ports.WithSyncOp(models.SyncOpFullSync)); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAgRules")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// DeleteIEAgAgRulesByIDs deletes IEAgAgRules by IDs
func (s *NetguardService) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteIEAgAgRulesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete IEAgAgRules")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// GetIEAgAgRulesByIDs returns a list of IEAgAgRules by IDs
func (s *NetguardService) GetIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var result []models.IEAgAgRule

	for _, id := range ids {
		rule, err := reader.GetIEAgAgRuleByID(ctx, id)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get IEAgAgRule %s", id.Key())
		}
		if rule != nil {
			result = append(result, *rule)
		}
	}

	return result, nil
}

// DeleteAddressGroupBindingPoliciesByIDs deletes address group binding policies by IDs
func (s *NetguardService) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Create validator
	validator := validation.NewAddressGroupBindingPolicyValidator(reader)

	// Check dependencies for each policy
	for _, id := range ids {
		if err := validator.CheckDependencies(ctx, id); err != nil {
			return err
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupBindingPoliciesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}
