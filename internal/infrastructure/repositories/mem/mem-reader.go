package mem

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type reader struct {
	registry *Registry
	ctx      context.Context
	writer   *writer // If not nil, use data from this writer instead of registry.db
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	var services map[string]models.Service
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
	} else {
		services = r.registry.db.GetServices()
	}

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all services in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, service := range services {
						if service.Namespace == id.Namespace {
							// Create a copy of the service to avoid modifying the original
							serviceCopy := service

							// Clear the address groups to avoid duplicates
							serviceCopy.AddressGroups = []models.AddressGroupRef{}

							// Populate address groups from bindings
							for _, binding := range bindings {
								if binding.ServiceRefKey() == serviceCopy.Key() {
									// Convert NamespacedObjectReference to AddressGroupRef
									agRef := models.NewAddressGroupRef(
										binding.AddressGroupRef.Name,
										models.WithNamespace(binding.AddressGroupRef.Namespace),
									)
									serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
								}
							}

							if err := consume(serviceCopy); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the service by exact key
				if service, ok := services[id.Key()]; ok {
					// Create a copy of the service to avoid modifying the original
					serviceCopy := service

					// Clear the address groups to avoid duplicates
					serviceCopy.AddressGroups = []models.AddressGroupRef{}

					// Populate address groups from bindings
					for _, binding := range bindings {
						if binding.ServiceRefKey() == serviceCopy.Key() {
							// Convert NamespacedObjectReference to AddressGroupRef
							agRef := models.NewAddressGroupRef(
								binding.AddressGroupRef.Name,
								models.WithNamespace(binding.AddressGroupRef.Namespace),
							)
							serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
						}
					}

					if err := consume(serviceCopy); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, service := range services {
		// Create a copy of the service to avoid modifying the original
		serviceCopy := service

		// Clear the address groups to avoid duplicates
		serviceCopy.AddressGroups = []models.AddressGroupRef{}

		// Populate address groups from bindings
		for _, binding := range bindings {
			if binding.ServiceRefKey() == serviceCopy.Key() {
				// Convert NamespacedObjectReference to AddressGroupRef
				agRef := models.NewAddressGroupRef(
					binding.AddressGroupRef.Name,
					models.WithNamespace(binding.AddressGroupRef.Namespace),
				)
				serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
			}
		}

		if err := consume(serviceCopy); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	var addressGroups map[string]models.AddressGroup

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroups != nil {
		addressGroups = r.writer.addressGroups
	} else {
		addressGroups = r.registry.db.GetAddressGroups()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all address groups in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, addressGroup := range addressGroups {
						if addressGroup.Namespace == id.Namespace {
							if err := consume(addressGroup); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the address group by exact key
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
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all bindings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, binding := range bindings {
						if binding.Namespace == id.Namespace {
							if err := consume(binding); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the binding by exact key
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
	var mappings map[string]models.AddressGroupPortMapping

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupPortMappings != nil {
		mappings = r.writer.addressGroupPortMappings
	} else {
		mappings = r.registry.db.GetAddressGroupPortMappings()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all port mappings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, mapping := range mappings {
						if mapping.Namespace == id.Namespace {
							if err := consume(mapping); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the port mapping by exact key
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
	var rules map[string]models.RuleS2S

	// Use data from writer if available
	if r.writer != nil && r.writer.ruleS2S != nil {
		rules = r.writer.ruleS2S
	} else {
		rules = r.registry.db.GetRuleS2S()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all rules in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, rule := range rules {
						if rule.Namespace == id.Namespace {
							if err := consume(rule); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the rule by exact key
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
	var aliases map[string]models.ServiceAlias

	// Use data from writer if available
	if r.writer != nil && r.writer.serviceAliases != nil {
		aliases = r.writer.serviceAliases
	} else {
		aliases = r.registry.db.GetServiceAliases()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all aliases in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, alias := range aliases {
						if alias.Namespace == id.Namespace {
							if err := consume(alias); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the alias by exact key
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

	var services map[string]models.Service
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
	} else {
		services = r.registry.db.GetServices()
	}

	if service, ok := services[id.Key()]; ok {
		// Create a copy of the service to avoid modifying the original
		serviceCopy := service

		// Clear the address groups to avoid duplicates
		serviceCopy.AddressGroups = []models.AddressGroupRef{}

		// Get all bindings to populate the address groups
		if r.writer != nil && r.writer.addressGroupBindings != nil {
			bindings = r.writer.addressGroupBindings
		} else {
			bindings = r.registry.db.GetAddressGroupBindings()
		}
		for _, binding := range bindings {
			// Check if the binding is for this service
			if binding.ServiceRefKey() == id.Key() {
				// Convert NamespacedObjectReference to AddressGroupRef
				agRef := models.NewAddressGroupRef(
					binding.AddressGroupRef.Name,
					models.WithNamespace(binding.AddressGroupRef.Namespace),
				)
				// Add the address group to the service
				serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
			}
		}

		return &serviceCopy, nil
	}
	return nil, ports.ErrNotFound
}

// GetAddressGroupByID gets an address group by ID
func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	var addressGroups map[string]models.AddressGroup

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroups != nil {
		addressGroups = r.writer.addressGroups
	} else {
		addressGroups = r.registry.db.GetAddressGroups()
	}

	if addressGroup, ok := addressGroups[id.Key()]; ok {
		return &addressGroup, nil
	}
	return nil, ports.ErrNotFound
}

// GetAddressGroupBindingByID gets an address group binding by ID
func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}

	requestedKey := id.Key()

	if binding, ok := bindings[requestedKey]; ok {
		return &binding, nil
	}
	return nil, ports.ErrNotFound
}

// GetAddressGroupPortMappingByID gets an address group port mapping by ID
func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	var mappings map[string]models.AddressGroupPortMapping

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupPortMappings != nil {
		mappings = r.writer.addressGroupPortMappings
	} else {
		mappings = r.registry.db.GetAddressGroupPortMappings()
	}

	if mapping, ok := mappings[id.Key()]; ok {
		return &mapping, nil
	}
	return nil, ports.ErrNotFound
}

// GetRuleS2SByID gets a rule s2s by ID
func (r *reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	var rules map[string]models.RuleS2S

	// Use data from writer if available
	if r.writer != nil && r.writer.ruleS2S != nil {
		rules = r.writer.ruleS2S
	} else {
		rules = r.registry.db.GetRuleS2S()
	}

	if rule, ok := rules[id.Key()]; ok {
		return &rule, nil
	}
	return nil, ports.ErrNotFound
}

// GetServiceAliasByID gets a service alias by ID
func (r *reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	var aliases map[string]models.ServiceAlias

	// Use data from writer if available
	if r.writer != nil && r.writer.serviceAliases != nil {
		aliases = r.writer.serviceAliases
	} else {
		aliases = r.registry.db.GetServiceAliases()
	}

	if alias, ok := aliases[id.Key()]; ok {
		return &alias, nil
	}
	return nil, ports.ErrNotFound
}

func (r *reader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	var policies map[string]models.AddressGroupBindingPolicy

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindingPolicies != nil {
		policies = r.writer.addressGroupBindingPolicies
	} else {
		policies = r.registry.db.GetAddressGroupBindingPolicies()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// Если установлен только namespace, возвращаем все политики в этом namespace
				if id.Name == "" && id.Namespace != "" {
					for _, policy := range policies {
						if policy.Namespace == id.Namespace {
							if err := consume(policy); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Иначе ищем политику по точному ключу
				if policy, ok := policies[id.Key()]; ok {
					if err := consume(policy); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, policy := range policies {
		if err := consume(policy); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	var policies map[string]models.AddressGroupBindingPolicy

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindingPolicies != nil {
		policies = r.writer.addressGroupBindingPolicies
	} else {
		policies = r.registry.db.GetAddressGroupBindingPolicies()
	}

	if policy, ok := policies[id.Key()]; ok {
		return &policy, nil
	}
	return nil, ports.ErrNotFound
}

func (r *reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	var rules map[string]models.IEAgAgRule

	// Use data from writer if available
	if r.writer != nil && r.writer.ieAgAgRules != nil {
		rules = r.writer.ieAgAgRules
	} else {
		rules = r.registry.db.GetIEAgAgRules()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all rules in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, rule := range rules {
						if rule.Namespace == id.Namespace {
							if err := consume(rule); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the rule by exact key
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

func (r *reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {

	var rules map[string]models.IEAgAgRule

	// Use data from writer if available
	if r.writer != nil && r.writer.ieAgAgRules != nil {
		rules = r.writer.ieAgAgRules
	} else {
		rules = r.registry.db.GetIEAgAgRules()
	}

	if rule, ok := rules[id.Key()]; ok {
		return &rule, nil
	}

	return nil, ports.ErrNotFound
}

func (r *reader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	var networks map[string]models.Network

	// Use data from writer if available
	if r.writer != nil && r.writer.networks != nil {
		networks = r.writer.networks
	} else {
		networks = r.registry.db.GetNetworks()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all networks in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, network := range networks {
						if network.Namespace == id.Namespace {
							if err := consume(network); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the network by exact key
				if network, ok := networks[id.Key()]; ok {
					if err := consume(network); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// If no scope or empty scope, return all networks
	for _, network := range networks {
		if err := consume(network); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader) GetNetworkByID(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {

	var networks map[string]models.Network

	// Use data from writer if available
	if r.writer != nil && r.writer.networks != nil {
		networks = r.writer.networks
	} else {
		networks = r.registry.db.GetNetworks()
	}

	if network, ok := networks[id.Key()]; ok {
		if network.BindingRef != nil {
		} else {
		}
		if network.AddressGroupRef != nil {
		} else {
		}
		return &network, nil
	}

	return nil, ports.ErrNotFound
}

// GetNetworkByCIDR gets a network by CIDR (for uniqueness validation)
func (r *reader) GetNetworkByCIDR(ctx context.Context, cidr string) (*models.Network, error) {

	var networks map[string]models.Network

	// Use data from writer if available
	if r.writer != nil && r.writer.networks != nil {
		networks = r.writer.networks
	} else {
		networks = r.registry.db.GetNetworks()
	}

	// Search through all networks to find matching CIDR
	for _, network := range networks {
		if network.CIDR == cidr {
			return &network, nil
		}
	}

	return nil, ports.ErrNotFound
}

func (r *reader) ListNetworkBindings(ctx context.Context, consume func(models.NetworkBinding) error, scope ports.Scope) error {
	var bindings map[string]models.NetworkBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.networkBindings != nil {
		bindings = r.writer.networkBindings
	} else {
		bindings = r.registry.db.GetNetworkBindings()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all network bindings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, binding := range bindings {
						if binding.Namespace == id.Namespace {
							if err := consume(binding); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the network binding by exact key
				if binding, ok := bindings[id.Key()]; ok {
					if err := consume(binding); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// If no scope or empty scope, return all network bindings
	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader) GetNetworkBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {

	var bindings map[string]models.NetworkBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.networkBindings != nil {
		bindings = r.writer.networkBindings
	} else {
		bindings = r.registry.db.GetNetworkBindings()
	}

	if binding, ok := bindings[id.Key()]; ok {
		return &binding, nil
	}

	return nil, ports.ErrNotFound
}

func (r *reader) ListHosts(ctx context.Context, consume func(models.Host) error, scope ports.Scope) error {
	var hosts map[string]models.Host

	// Use data from writer if available
	if r.writer != nil && r.writer.hosts != nil {
		hosts = r.writer.hosts
	} else {
		hosts = r.registry.db.GetHosts()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all hosts in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, host := range hosts {
						if host.Namespace == id.Namespace {
							if err := consume(host); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the host by exact key
				if host, ok := hosts[id.Key()]; ok {
					if err := consume(host); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// If no scope or empty scope, return all hosts
	for _, host := range hosts {
		if err := consume(host); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader) GetHostByID(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	var hosts map[string]models.Host

	// Use data from writer if available
	if r.writer != nil && r.writer.hosts != nil {
		hosts = r.writer.hosts
	} else {
		hosts = r.registry.db.GetHosts()
	}

	if host, ok := hosts[id.Key()]; ok {
		return &host, nil
	}

	return nil, ports.ErrNotFound
}

func (r *reader) ListHostBindings(ctx context.Context, consume func(models.HostBinding) error, scope ports.Scope) error {
	var hostBindings map[string]models.HostBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.hostBindings != nil {
		hostBindings = r.writer.hostBindings
	} else {
		hostBindings = r.registry.db.GetHostBindings()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all host bindings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, hostBinding := range hostBindings {
						if hostBinding.Namespace == id.Namespace {
							if err := consume(hostBinding); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the host binding by exact key
				if hostBinding, ok := hostBindings[id.Key()]; ok {
					if err := consume(hostBinding); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// If no scope or empty scope, return all host bindings
	for _, hostBinding := range hostBindings {
		if err := consume(hostBinding); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	var hostBindings map[string]models.HostBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.hostBindings != nil {
		hostBindings = r.writer.hostBindings
	} else {
		hostBindings = r.registry.db.GetHostBindings()
	}

	if hostBinding, ok := hostBindings[id.Key()]; ok {
		return &hostBinding, nil
	}

	return nil, ports.ErrNotFound
}
