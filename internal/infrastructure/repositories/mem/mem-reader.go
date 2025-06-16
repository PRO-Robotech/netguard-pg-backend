package mem

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type reader struct {
	registry *Registry
	ctx      context.Context
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	services := r.registry.db.GetServices()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if service, ok := services[id.Key()]; ok {
					if err := consume(service); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, service := range services {
		if err := consume(service); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	addressGroups := r.registry.db.GetAddressGroups()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if addressGroup, ok := addressGroups[id.Key()]; ok {
					if err := consume(addressGroup); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, addressGroup := range addressGroups {
		if err := consume(addressGroup); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	bindings := r.registry.db.GetAddressGroupBindings()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if binding, ok := bindings[id.Key()]; ok {
					if err := consume(binding); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	mappings := r.registry.db.GetAddressGroupPortMappings()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if mapping, ok := mappings[id.Key()]; ok {
					if err := consume(mapping); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, mapping := range mappings {
		if err := consume(mapping); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	rules := r.registry.db.GetRuleS2S()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if rule, ok := rules[id.Key()]; ok {
					if err := consume(rule); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	aliases := r.registry.db.GetServiceAliases()
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				if alias, ok := aliases[id.Key()]; ok {
					if err := consume(alias); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, alias := range aliases {
		if err := consume(alias); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	status := r.registry.db.GetSyncStatus()
	return &status, nil
}

// GetServiceByID gets a service by ID
func (r *reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	services := r.registry.db.GetServices()
	if service, ok := services[id.Key()]; ok {
		return &service, nil
	}
	return nil, nil
}

// GetAddressGroupByID gets an address group by ID
func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	addressGroups := r.registry.db.GetAddressGroups()
	if addressGroup, ok := addressGroups[id.Key()]; ok {
		return &addressGroup, nil
	}
	return nil, nil
}

// GetAddressGroupBindingByID gets an address group binding by ID
func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	bindings := r.registry.db.GetAddressGroupBindings()
	if binding, ok := bindings[id.Key()]; ok {
		return &binding, nil
	}
	return nil, nil
}

// GetAddressGroupPortMappingByID gets an address group port mapping by ID
func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	mappings := r.registry.db.GetAddressGroupPortMappings()
	if mapping, ok := mappings[id.Key()]; ok {
		return &mapping, nil
	}
	return nil, nil
}

// GetRuleS2SByID gets a rule s2s by ID
func (r *reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	rules := r.registry.db.GetRuleS2S()
	if rule, ok := rules[id.Key()]; ok {
		return &rule, nil
	}
	return nil, nil
}

// GetServiceAliasByID gets a service alias by ID
func (r *reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	aliases := r.registry.db.GetServiceAliases()
	if alias, ok := aliases[id.Key()]; ok {
		return &alias, nil
	}
	return nil, nil
}
