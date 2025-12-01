package mem

import (
	"context"
	"net"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type reader struct {
	registry *Registry
	ctx      context.Context
	writer   *writer
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	var services map[string]models.Service

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
	} else {
		services = r.registry.db.GetServices()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all services in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, svc := range services {
						if svc.Namespace == id.Namespace {
							if err := consume(svc); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the service by exact key
				if svc, ok := services[id.Key()]; ok {
					if err := consume(svc); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, svc := range services {
		if err := consume(svc); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	var services map[string]models.Service

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
	} else {
		services = r.registry.db.GetServices()
	}

	if svc, ok := services[id.Key()]; ok {
		// Load address groups for this service
		var addressGroups []models.AddressGroupRef

		// Use bindings data from writer if available
		var bindings map[string]models.AddressGroupBinding
		if r.writer != nil && r.writer.addressGroupBindings != nil {
			bindings = r.writer.addressGroupBindings
		} else {
			bindings = r.registry.db.GetAddressGroupBindings()
		}

		for _, binding := range bindings {
			if binding.ServiceRef.Name == id.Name && binding.ServiceRef.Namespace == id.Namespace {
				addressGroups = append(addressGroups, binding.AddressGroupRef)
			}
		}

		svcCopy := svc
		svcCopy.AddressGroups = addressGroups
		return &svcCopy, nil
	}

	return nil, ports.ErrNotFound
}

func (r *reader) ListSvcSvcRules(ctx context.Context, consume func(models.SvcSvcRule) error, scope ports.Scope) error {
	// No in-memory storage for svc-svc rules in tests; behave as empty.
	return nil
}

func (r *reader) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	// In-memory registry does not currently persist SvcSvcRule objects.
	// Return ErrNotFound to match behaviour expected by higher layers when
	// a rule isn't present in the store.
	return nil, ports.ErrNotFound
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
					for _, ag := range addressGroups {
						if ag.Namespace == id.Namespace {
							if err := consume(ag); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the address group by exact key
				if ag, ok := addressGroups[id.Key()]; ok {
					if err := consume(ag); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, ag := range addressGroups {
		if err := consume(ag); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {

	var addressGroups map[string]models.AddressGroup

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroups != nil {
		addressGroups = r.writer.addressGroups
	} else {
		addressGroups = r.registry.db.GetAddressGroups()
	}

	if ag, ok := addressGroups[id.Key()]; ok {
		return &ag, nil
	}

	return nil, ports.ErrNotFound
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

func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {

	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}

	if binding, ok := bindings[id.Key()]; ok {
		return &binding, nil
	}

	return nil, ports.ErrNotFound
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

func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {

	var mappings map[string]models.AddressGroupPortMapping

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

func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	status := r.registry.db.GetSyncStatus()
	return &status, nil
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

func (r *reader) ListSvcFqdnRules(ctx context.Context, consume func(models.SvcFqdnRule) error, scope ports.Scope) error {
	// Stub implementation - return empty list
	return nil
}

func (r *reader) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	// Stub implementation - always return not found
	return nil, ports.ErrNotFound
}

func (r *reader) ListNetworks(ctx context.Context, consume func(models.Network) error, scope ports.Scope) error {
	var networks map[string]models.Network

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

// GetNetworksOverlappingCIDR gets networks with overlapping CIDR ranges
func (r *reader) GetNetworksOverlappingCIDR(ctx context.Context, cidr string) ([]*models.Network, error) {
	var networks map[string]models.Network

	if r.writer != nil && r.writer.networks != nil {
		networks = r.writer.networks
	} else {
		networks = r.registry.db.GetNetworks()
	}

	_, inputNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var overlapping []*models.Network

	// Check each network for overlap
	for _, network := range networks {
		_, networkNet, err := net.ParseCIDR(network.CIDR)
		if err != nil {
			continue
		}

		if cidrsOverlap(inputNet, networkNet) {
			networkCopy := network
			overlapping = append(overlapping, &networkCopy)
		}
	}

	return overlapping, nil
}

func cidrsOverlap(a, b *net.IPNet) bool {
	if a.Contains(b.IP) {
		return true
	}

	if b.Contains(a.IP) {
		return true
	}

	bBroadcast := broadcastIP(b)
	if a.Contains(bBroadcast) {
		return true
	}

	aBroadcast := broadcastIP(a)
	if b.Contains(aBroadcast) {
		return true
	}

	return false
}

func broadcastIP(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	if ip == nil {
		ip = n.IP.To16()
		if ip == nil {
			return n.IP
		}
	}

	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^n.Mask[i]
	}
	return broadcast
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
	var bindings map[string]models.HostBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.hostBindings != nil {
		bindings = r.writer.hostBindings
	} else {
		bindings = r.registry.db.GetHostBindings()
	}

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all host bindings in that namespace
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

func (r *reader) GetHostBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	var bindings map[string]models.HostBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.hostBindings != nil {
		bindings = r.writer.hostBindings
	} else {
		bindings = r.registry.db.GetHostBindings()
	}

	if binding, ok := bindings[id.Key()]; ok {
		return &binding, nil
	}

	return nil, ports.ErrNotFound
}
