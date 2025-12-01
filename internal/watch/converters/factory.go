package converters

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/registry/convert"
)

// BuildAllConverters returns converters for every resource type that supports watch.
func BuildAllConverters(facade *services.NetguardFacade) []ResourceConverter {
	return []ResourceConverter{
		newServiceConverter(facade),
		newAddressGroupConverter(facade),
		newAddressGroupBindingConverter(facade),
		newAddressGroupPortMappingConverter(facade),
		newAddressGroupBindingPolicyConverter(facade),
		newHostConverter(facade),
		newHostBindingConverter(facade),
		newNetworkConverter(facade),
		newNetworkBindingConverter(facade),
		newSvcSvcRuleConverter(facade),
		newSvcFqdnRuleConverter(facade),
	}
}

func newServiceConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.ServiceConverter{}
	return newDomainConverter[models.Service](
		"services",
		facade.GetServiceByID,
		func(ctx context.Context) ([]models.Service, error) {
			return facade.GetServices(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.Service) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newAddressGroupConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.AddressGroupConverter{}
	return newDomainConverter[models.AddressGroup](
		"address_groups",
		facade.GetAddressGroupByID,
		func(ctx context.Context) ([]models.AddressGroup, error) {
			return facade.GetAddressGroups(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.AddressGroup) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newAddressGroupBindingConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.AddressGroupBindingConverter{}
	return newDomainConverter[models.AddressGroupBinding](
		"address_group_bindings",
		facade.GetAddressGroupBindingByID,
		func(ctx context.Context) ([]models.AddressGroupBinding, error) {
			return facade.GetAddressGroupBindings(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.AddressGroupBinding) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newAddressGroupPortMappingConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.AddressGroupPortMappingConverter{}
	return newDomainConverter[models.AddressGroupPortMapping](
		"address_group_port_mappings",
		facade.GetAddressGroupPortMappingByID,
		func(ctx context.Context) ([]models.AddressGroupPortMapping, error) {
			return facade.GetAddressGroupPortMappings(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.AddressGroupPortMapping) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newAddressGroupBindingPolicyConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.AddressGroupBindingPolicyConverter{}
	return newDomainConverter[models.AddressGroupBindingPolicy](
		"address_group_binding_policies",
		facade.GetAddressGroupBindingPolicyByID,
		func(ctx context.Context) ([]models.AddressGroupBindingPolicy, error) {
			return facade.GetAddressGroupBindingPolicies(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.AddressGroupBindingPolicy) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newHostConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.HostConverter{}
	return newDomainConverter[models.Host](
		"hosts",
		facade.GetHostByID,
		func(ctx context.Context) ([]models.Host, error) {
			return facade.GetHosts(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.Host) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newHostBindingConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.HostBindingConverter{}
	return newDomainConverter[models.HostBinding](
		"host_bindings",
		facade.GetHostBindingByID,
		func(ctx context.Context) ([]models.HostBinding, error) {
			return facade.GetHostBindings(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.HostBinding) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newNetworkConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.NetworkConverter{}
	return newDomainConverter[models.Network](
		"networks",
		facade.GetNetworkByID,
		func(ctx context.Context) ([]models.Network, error) {
			return facade.GetNetworks(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.Network) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newNetworkBindingConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.NetworkBindingConverter{}
	return newDomainConverter[models.NetworkBinding](
		"network_bindings",
		facade.GetNetworkBindingByID,
		func(ctx context.Context) ([]models.NetworkBinding, error) {
			return facade.GetNetworkBindings(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.NetworkBinding) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newSvcSvcRuleConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.SvcSvcRuleConverter{}
	return newDomainConverter[models.SvcSvcRule](
		"svc_svc_rules",
		facade.GetSvcSvcRuleByID,
		func(ctx context.Context) ([]models.SvcSvcRule, error) {
			return facade.GetSvcSvcRules(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.SvcSvcRule) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}

func newSvcFqdnRuleConverter(facade *services.NetguardFacade) ResourceConverter {
	conv := &convert.SvcFqdnRuleConverter{}
	return newDomainConverter[models.SvcFqdnRule](
		"svc_fqdn_rules",
		facade.GetSvcFqdnRuleByID,
		func(ctx context.Context) ([]models.SvcFqdnRule, error) {
			return facade.GetSvcFqdnRules(ctx, ports.EmptyScope{})
		},
		func(ctx context.Context, domainObj *models.SvcFqdnRule) (runtime.Object, error) {
			return conv.FromDomain(ctx, domainObj)
		},
	)
}
