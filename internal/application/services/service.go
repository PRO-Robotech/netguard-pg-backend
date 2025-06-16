package services

import (
	"context"

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
		if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
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

	if err = writer.SyncServices(ctx, services, scope); err != nil {
		return errors.Wrap(err, "failed to sync services")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
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
		if err := addressGroupValidator.ValidateForCreation(ctx, addressGroup); err != nil {
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

	if err = writer.SyncAddressGroups(ctx, addressGroups, scope); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
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
	for _, binding := range bindings {
		if err := bindingValidator.ValidateForCreation(ctx, binding); err != nil {
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

	if err = writer.SyncAddressGroupBindings(ctx, bindings, scope); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// SyncAddressGroupPortMappings syncs address group port mappings
func (s *NetguardService) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope) error {
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
		if err := mappingValidator.ValidateForCreation(ctx, mapping); err != nil {
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

	if err = writer.SyncAddressGroupPortMappings(ctx, mappings, scope); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}
	if err = writer.Commit(); err != nil {
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
		if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
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

	if err = writer.SyncRuleS2S(ctx, rules, scope); err != nil {
		return errors.Wrap(err, "failed to sync rule s2s")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
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
	for _, alias := range aliases {
		if err := aliasValidator.ValidateForCreation(ctx, alias); err != nil {
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

	if err = writer.SyncServiceAliases(ctx, aliases, scope); err != nil {
		return errors.Wrap(err, "failed to sync service aliases")
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
	// Note: Address group bindings don't have dependencies, so we don't need to check for them
	// However, we could add validation to ensure the bindings exist before deleting them

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteAddressGroupBindingsByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings")
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

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

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
