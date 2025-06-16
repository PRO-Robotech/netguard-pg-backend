package mem

import (
	"context"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type writer struct {
	registry                 *Registry
	ctx                      context.Context
	services                 map[string]models.Service
	addressGroups            map[string]models.AddressGroup
	addressGroupBindings     map[string]models.AddressGroupBinding
	addressGroupPortMappings map[string]models.AddressGroupPortMapping
	ruleS2S                  map[string]models.RuleS2S
	serviceAliases           map[string]models.ServiceAlias
}

func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	if w.services == nil {
		w.services = make(map[string]models.Service)
	}
	for _, service := range services {
		w.services[service.Key()] = service
	}
	return nil
}

func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroups == nil {
		w.addressGroups = make(map[string]models.AddressGroup)
	}
	for _, addressGroup := range addressGroups {
		w.addressGroups[addressGroup.Key()] = addressGroup
	}
	return nil
}

func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroupBindings == nil {
		w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
	}
	for _, binding := range bindings {
		key := models.NewResourceIdentifier(binding.Name, models.WithNamespace(binding.Namespace))
		w.addressGroupBindings[key.Key()] = binding
	}
	return nil
}

func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	if w.addressGroupPortMappings == nil {
		w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
	}
	for _, mapping := range mappings {
		w.addressGroupPortMappings[mapping.Key()] = mapping
	}
	return nil
}

func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	if w.ruleS2S == nil {
		w.ruleS2S = make(map[string]models.RuleS2S)
	}
	for _, rule := range rules {
		w.ruleS2S[rule.Key()] = rule
	}
	return nil
}

func (w *writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	if w.serviceAliases == nil {
		w.serviceAliases = make(map[string]models.ServiceAlias)
	}
	for _, alias := range aliases {
		w.serviceAliases[alias.Key()] = alias
	}
	return nil
}

func (w *writer) Commit() error {
	if w.services != nil {
		w.registry.db.SetServices(w.services)
	}
	if w.serviceAliases != nil {
		w.registry.db.SetServiceAliases(w.serviceAliases)
	}
	if w.addressGroups != nil {
		w.registry.db.SetAddressGroups(w.addressGroups)
	}
	if w.addressGroupBindings != nil {
		w.registry.db.SetAddressGroupBindings(w.addressGroupBindings)
	}
	if w.addressGroupPortMappings != nil {
		w.registry.db.SetAddressGroupPortMappings(w.addressGroupPortMappings)
	}
	if w.ruleS2S != nil {
		w.registry.db.SetRuleS2S(w.ruleS2S)
	}
	w.registry.db.SetSyncStatus(models.SyncStatus{
		UpdatedAt: time.Now(),
	})
	return nil
}

// DeleteServicesByIDs deletes services by IDs
func (w *writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.services == nil {
		w.services = make(map[string]models.Service)
		// Copy existing services
		for k, v := range w.registry.db.GetServices() {
			w.services[k] = v
		}
	}

	for _, id := range ids {
		delete(w.services, id.Key())
	}

	return nil
}

// DeleteAddressGroupsByIDs deletes address groups by IDs
func (w *writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroups == nil {
		w.addressGroups = make(map[string]models.AddressGroup)
		// Copy existing address groups
		for k, v := range w.registry.db.GetAddressGroups() {
			w.addressGroups[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroups, id.Key())
	}

	return nil
}

// DeleteAddressGroupBindingsByIDs deletes address group bindings by IDs
func (w *writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroupBindings == nil {
		w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
		// Copy existing address group bindings
		for k, v := range w.registry.db.GetAddressGroupBindings() {
			w.addressGroupBindings[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroupBindings, id.Key())
	}

	return nil
}

// DeleteAddressGroupPortMappingsByIDs deletes address group port mappings by IDs
func (w *writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroupPortMappings == nil {
		w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
		// Copy existing address group port mappings
		for k, v := range w.registry.db.GetAddressGroupPortMappings() {
			w.addressGroupPortMappings[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroupPortMappings, id.Key())
	}

	return nil
}

// DeleteRuleS2SByIDs deletes rule s2s by IDs
func (w *writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.ruleS2S == nil {
		w.ruleS2S = make(map[string]models.RuleS2S)
		// Copy existing rule s2s
		for k, v := range w.registry.db.GetRuleS2S() {
			w.ruleS2S[k] = v
		}
	}

	for _, id := range ids {
		delete(w.ruleS2S, id.Key())
	}

	return nil
}

// DeleteServiceAliasesByIDs deletes service aliases by IDs
func (w *writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.serviceAliases == nil {
		w.serviceAliases = make(map[string]models.ServiceAlias)
		// Copy existing service aliases
		for k, v := range w.registry.db.GetServiceAliases() {
			w.serviceAliases[k] = v
		}
	}

	for _, id := range ids {
		delete(w.serviceAliases, id.Key())
	}

	return nil
}

func (w *writer) Abort() {
	w.services = nil
	w.addressGroups = nil
	w.addressGroupBindings = nil
	w.addressGroupPortMappings = nil
	w.ruleS2S = nil
	w.serviceAliases = nil
}
