package client

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type MockBackendClient struct {
	services             []models.Service
	addressGroups        []models.AddressGroup
	addressGroupBindings []models.AddressGroupBinding
	networks             []models.Network
	networkBindings      []models.NetworkBinding
	hosts                []models.Host
	hostBindings         []models.HostBinding
}

func NewMockBackendClient() *MockBackendClient {
	return &MockBackendClient{
		services: []models.Service{
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-service-1",
						models.WithNamespace("netguard-test"),
					),
				},
				Description: "Test Service 1",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "80",
						Description: "HTTP port",
					},
				},
			},
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-service-2",
						models.WithNamespace("netguard-test"),
					),
				},
				Description: "Test Service 2",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "443",
						Description: "HTTPS port",
					},
				},
			},
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-service",
						models.WithNamespace("default"),
					),
				},
				Description: "Test Service for K8s Service Registry Tests",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "8080",
						Description: "Test port",
					},
				},
				AddressGroups: []models.AddressGroupRef{
					models.NewAddressGroupRef(
						"test-addressgroup-default",
						models.WithNamespace("default"),
					),
				},
			},
		},
		addressGroups: []models.AddressGroup{
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-addressgroup-1",
						models.WithNamespace("netguard-test"),
					),
				},
				DefaultAction: models.ActionAccept,
				Logs:          true,
				Trace:         false,
			},
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-addressgroup-2",
						models.WithNamespace("netguard-test"),
					),
				},
				DefaultAction: models.ActionDrop,
				Logs:          false,
				Trace:         true,
			},
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-addressgroup-default",
						models.WithNamespace("default"),
					),
				},
				DefaultAction: models.ActionAccept,
				Logs:          false,
				Trace:         false,
			},
		},
		addressGroupBindings: []models.AddressGroupBinding{
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-binding-1",
						models.WithNamespace("netguard-test"),
					),
				},
			},
		},
		hosts:        []models.Host{},
		hostBindings: []models.HostBinding{},
	}
}
func (m *MockBackendClient) GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	for _, service := range m.services {
		if service.ResourceIdentifier.Key() == id.Key() {
			return &service, nil
		}
	}
	return nil, fmt.Errorf("service not found: %s", id.Key())
}
func (m *MockBackendClient) ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	return m.services, nil
}
func (m *MockBackendClient) CreateService(ctx context.Context, service *models.Service) error {
	m.services = append(m.services, *service)
	return nil
}
func (m *MockBackendClient) UpdateService(ctx context.Context, service *models.Service) error {
	for i, svc := range m.services {
		if svc.ResourceIdentifier.Key() == service.ResourceIdentifier.Key() {
			m.services[i] = *service
			return nil
		}
	}
	return fmt.Errorf("service not found for update: %s", service.ResourceIdentifier.Key())
}
func (m *MockBackendClient) DeleteService(ctx context.Context, id models.ResourceIdentifier) error {
	for i, service := range m.services {
		if service.ResourceIdentifier.Key() == id.Key() {
			m.services = append(m.services[:i], m.services[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("service not found for delete: %s", id.Key())
}
func (m *MockBackendClient) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	for _, group := range m.addressGroups {
		if group.ResourceIdentifier.Key() == id.Key() {
			return &group, nil
		}
	}
	return nil, fmt.Errorf("address group not found: %s", id.Key())
}
func (m *MockBackendClient) ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	return m.addressGroups, nil
}
func (m *MockBackendClient) CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	m.addressGroups = append(m.addressGroups, *group)
	return nil
}
func (m *MockBackendClient) UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error {
	for i, ag := range m.addressGroups {
		if ag.ResourceIdentifier.Key() == group.ResourceIdentifier.Key() {
			m.addressGroups[i] = *group
			return nil
		}
	}
	return fmt.Errorf("address group not found for update: %s", group.ResourceIdentifier.Key())
}
func (m *MockBackendClient) DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error {
	for i, group := range m.addressGroups {
		if group.ResourceIdentifier.Key() == id.Key() {
			m.addressGroups = append(m.addressGroups[:i], m.addressGroups[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("address group not found for delete: %s", id.Key())
}
func (m *MockBackendClient) GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	for _, binding := range m.addressGroupBindings {
		if binding.ResourceIdentifier.Key() == id.Key() {
			return &binding, nil
		}
	}
	return nil, fmt.Errorf("binding not found: %s", id.Key())
}
func (m *MockBackendClient) ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	return m.addressGroupBindings, nil
}
func (m *MockBackendClient) CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	m.addressGroupBindings = append(m.addressGroupBindings, *binding)
	return nil
}
func (m *MockBackendClient) UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error {
	for i, b := range m.addressGroupBindings {
		if b.ResourceIdentifier.Key() == binding.ResourceIdentifier.Key() {
			m.addressGroupBindings[i] = *binding
			return nil
		}
	}
	return fmt.Errorf("binding not found for update: %s", binding.ResourceIdentifier.Key())
}
func (m *MockBackendClient) DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error {
	for i, binding := range m.addressGroupBindings {
		if binding.ResourceIdentifier.Key() == id.Key() {
			m.addressGroupBindings = append(m.addressGroupBindings[:i], m.addressGroupBindings[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("binding not found for delete: %s", id.Key())
}
func (m *MockBackendClient) GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	return nil, nil
}
func (m *MockBackendClient) CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return nil
}
func (m *MockBackendClient) UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error {
	return nil
}
func (m *MockBackendClient) DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
func (m *MockBackendClient) GetRuleS2S(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) ListRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	return nil, nil
}
func (m *MockBackendClient) CreateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	return nil
}
func (m *MockBackendClient) UpdateRuleS2S(ctx context.Context, rule *models.RuleS2S) error {
	return nil
}
func (m *MockBackendClient) DeleteRuleS2S(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
func (m *MockBackendClient) GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	return nil, nil
}
func (m *MockBackendClient) CreateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	return nil
}
func (m *MockBackendClient) UpdateServiceAlias(ctx context.Context, alias *models.ServiceAlias) error {
	return nil
}
func (m *MockBackendClient) DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
func (m *MockBackendClient) GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error) {
	return nil, nil
}
func (m *MockBackendClient) CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return nil
}
func (m *MockBackendClient) UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	return nil
}
func (m *MockBackendClient) DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
func (m *MockBackendClient) GetIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) ListIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	return nil, nil
}
func (m *MockBackendClient) CreateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	return nil
}
func (m *MockBackendClient) UpdateIEAgAgRule(ctx context.Context, rule *models.IEAgAgRule) error {
	return nil
}
func (m *MockBackendClient) DeleteIEAgAgRule(ctx context.Context, id models.ResourceIdentifier) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) GetNetwork(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	for _, network := range m.networks {
		if network.Key() == id.Key() {
			return &network, nil
		}
	}
	return nil, fmt.Errorf("network not found: %s", id.Key())
}
func (m *MockBackendClient) ListNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	return m.networks, nil
}
func (m *MockBackendClient) CreateNetwork(ctx context.Context, network *models.Network) error {
	m.networks = append(m.networks, *network)
	return nil
}
func (m *MockBackendClient) UpdateNetwork(ctx context.Context, network *models.Network) error {
	for i, existing := range m.networks {
		if existing.Key() == network.Key() {
			m.networks[i] = *network
			return nil
		}
	}
	return fmt.Errorf("network not found: %s", network.Key())
}
func (m *MockBackendClient) DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error {
	for i, network := range m.networks {
		if network.Key() == id.Key() {
			m.networks = append(m.networks[:i], m.networks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("network not found: %s", id.Key())
}
func (m *MockBackendClient) GetNetworkBinding(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	for _, binding := range m.networkBindings {
		if binding.Key() == id.Key() {
			return &binding, nil
		}
	}
	return nil, fmt.Errorf("network binding not found: %s", id.Key())
}
func (m *MockBackendClient) ListNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	return m.networkBindings, nil
}
func (m *MockBackendClient) CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	m.networkBindings = append(m.networkBindings, *binding)
	return nil
}
func (m *MockBackendClient) UpdateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	for i, existing := range m.networkBindings {
		if existing.Key() == binding.Key() {
			m.networkBindings[i] = *binding
			return nil
		}
	}
	return fmt.Errorf("network binding not found: %s", binding.Key())
}
func (m *MockBackendClient) DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error {
	for i, binding := range m.networkBindings {
		if binding.Key() == id.Key() {
			m.networkBindings = append(m.networkBindings[:i], m.networkBindings[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("network binding not found: %s", id.Key())
}
func (m *MockBackendClient) Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error {
	return nil
}
func (m *MockBackendClient) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return &models.SyncStatus{}, nil
}
func (m *MockBackendClient) GetDependencyValidator() *validation.DependencyValidator {
	return validation.NewDependencyValidator(nil)
}
func (m *MockBackendClient) GetReader(ctx context.Context) (ports.Reader, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *MockBackendClient) HealthCheck(ctx context.Context) error {
	return nil
}
func (m *MockBackendClient) Ping(ctx context.Context) error {
	return nil
}
func (m *MockBackendClient) UpdateServiceMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return m.UpdateService(ctx, &models.Service{SelfRef: models.SelfRef{ResourceIdentifier: id}, Meta: meta})
}
func (m *MockBackendClient) UpdateAddressGroupMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return m.UpdateAddressGroup(ctx, &models.AddressGroup{SelfRef: models.SelfRef{ResourceIdentifier: id}, Meta: meta})
}
func (m *MockBackendClient) UpdateAddressGroupBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return m.UpdateAddressGroupBinding(ctx, &models.AddressGroupBinding{SelfRef: models.SelfRef{ResourceIdentifier: id}, Meta: meta})
}
func (m *MockBackendClient) UpdateAddressGroupPortMappingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil
}
func (m *MockBackendClient) UpdateRuleS2SMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil
}
func (m *MockBackendClient) UpdateServiceAliasMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil
}
func (m *MockBackendClient) UpdateAddressGroupBindingPolicyMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil
}
func (m *MockBackendClient) UpdateIEAgAgRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) UpdateNetworkMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	for i, network := range m.networks {
		if network.Key() == id.Key() {
			m.networks[i].Meta = meta
			return nil
		}
	}
	return fmt.Errorf("network not found: %s", id.Key())
}
func (m *MockBackendClient) UpdateNetworkBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	for i, binding := range m.networkBindings {
		if binding.Key() == id.Key() {
			m.networkBindings[i].Meta = meta
			return nil
		}
	}
	return fmt.Errorf("network binding not found: %s", id.Key())
}
func (m *MockBackendClient) ListAddressGroupsForService(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.AddressGroup, error) {
	if serviceID.Name == "test-service-1" {
		return []models.AddressGroup{
			{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"test-addressgroup-1",
						models.WithNamespace("netguard-test"),
					),
				},
				DefaultAction: models.ActionAccept,
				Logs:          true,
				Trace:         false,
			},
		}, nil
	}
	return []models.AddressGroup{}, nil
}
func (m *MockBackendClient) ListRuleS2SDstOwnRef(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.RuleS2S, error) {
	return []models.RuleS2S{
		{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(
					"test-rule-cross-ns",
					models.WithNamespace("other-namespace"),
				),
			},
			ServiceLocalRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "local-service",
				},
				Namespace: "other-namespace",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       serviceID.Name,
				},
				Namespace: serviceID.Namespace,
			},
			Traffic: models.INGRESS,
		},
	}, nil
}
func (m *MockBackendClient) ListAccessPorts(ctx context.Context, mappingID models.ResourceIdentifier) ([]models.ServicePortsRef, error) {
	return []models.ServicePortsRef{
		{
			ServiceRef: models.NewServiceRef("test-service-1", models.WithNamespace("netguard-test")),
			Ports: models.ServicePorts{
				Ports: map[models.TransportProtocol][]models.PortRange{
					models.TCP: {
						{Start: 80, End: 80},
						{Start: 443, End: 443},
					},
				},
			},
		},
	}, nil
}
func (m *MockBackendClient) GetHost(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error) {
	for _, host := range m.hosts {
		if host.ResourceIdentifier.Key() == id.Key() {
			return &host, nil
		}
	}
	return nil, fmt.Errorf("host not found: %s", id.Key())
}
func (m *MockBackendClient) ListHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error) {
	return m.hosts, nil
}
func (m *MockBackendClient) CreateHost(ctx context.Context, host *models.Host) error {
	m.hosts = append(m.hosts, *host)
	return nil
}
func (m *MockBackendClient) UpdateHost(ctx context.Context, host *models.Host) error {
	for i, existingHost := range m.hosts {
		if existingHost.ResourceIdentifier.Key() == host.ResourceIdentifier.Key() {
			m.hosts[i] = *host
			return nil
		}
	}
	return fmt.Errorf("host not found for update: %s", host.ResourceIdentifier.Key())
}
func (m *MockBackendClient) DeleteHost(ctx context.Context, id models.ResourceIdentifier) error {
	for i, host := range m.hosts {
		if host.ResourceIdentifier.Key() == id.Key() {
			m.hosts = append(m.hosts[:i], m.hosts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("host not found for deletion: %s", id.Key())
}
func (m *MockBackendClient) GetHostBinding(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {
	for _, binding := range m.hostBindings {
		if binding.ResourceIdentifier.Key() == id.Key() {
			return &binding, nil
		}
	}
	return nil, fmt.Errorf("host binding not found: %s", id.Key())
}
func (m *MockBackendClient) ListHostBindings(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error) {
	return m.hostBindings, nil
}
func (m *MockBackendClient) CreateHostBinding(ctx context.Context, binding *models.HostBinding) error {
	m.hostBindings = append(m.hostBindings, *binding)
	return nil
}
func (m *MockBackendClient) UpdateHostBinding(ctx context.Context, binding *models.HostBinding) error {
	for i, existingBinding := range m.hostBindings {
		if existingBinding.ResourceIdentifier.Key() == binding.ResourceIdentifier.Key() {
			m.hostBindings[i] = *binding
			return nil
		}
	}
	return fmt.Errorf("host binding not found for update: %s", binding.ResourceIdentifier.Key())
}
func (m *MockBackendClient) DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error {
	for i, binding := range m.hostBindings {
		if binding.ResourceIdentifier.Key() == id.Key() {
			m.hostBindings = append(m.hostBindings[:i], m.hostBindings[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("host binding not found for deletion: %s", id.Key())
}
func (m *MockBackendClient) UpdateHostMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	for i, host := range m.hosts {
		if host.ResourceIdentifier.Key() == id.Key() {
			m.hosts[i].Meta = meta
			return nil
		}
	}
	return fmt.Errorf("host not found for meta update: %s", id.Key())
}
func (m *MockBackendClient) UpdateHostBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	for i, binding := range m.hostBindings {
		if binding.ResourceIdentifier.Key() == id.Key() {
			m.hostBindings[i].Meta = meta
			return nil
		}
	}
	return fmt.Errorf("host binding not found for meta update: %s", id.Key())
}
func (m *MockBackendClient) Close() error {
	return nil
}
func createMockGRPCBackendClient(config BackendClientConfig) (BackendClient, error) {
	return NewMockBackendClient(), nil
}
func createMockCircuitBreakerClient(client BackendClient, config BackendClientConfig) BackendClient {
	return client
}
func createMockCachedBackendClient(client BackendClient, config BackendClientConfig) BackendClient {
	return client
}
func (m *MockBackendClient) GetSvcSvcRule(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *MockBackendClient) ListSvcSvcRules(ctx context.Context, scope ports.Scope) ([]models.SvcSvcRule, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *MockBackendClient) CreateSvcSvcRule(ctx context.Context, rule *models.SvcSvcRule) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) UpdateSvcSvcRule(ctx context.Context, rule *models.SvcSvcRule) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) DeleteSvcSvcRule(ctx context.Context, id models.ResourceIdentifier) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) UpdateSvcSvcRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) GetSvcFqdnRule(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *MockBackendClient) ListSvcFqdnRules(ctx context.Context, scope ports.Scope) ([]models.SvcFqdnRule, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *MockBackendClient) CreateSvcFqdnRule(ctx context.Context, rule *models.SvcFqdnRule) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) UpdateSvcFqdnRule(ctx context.Context, rule *models.SvcFqdnRule) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) DeleteSvcFqdnRule(ctx context.Context, id models.ResourceIdentifier) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) UpdateSvcFqdnRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return fmt.Errorf("not implemented")
}
func (m *MockBackendClient) MarkForDeletion(ctx context.Context, namespace, name, kind string) error {
	return fmt.Errorf("not implemented")
}

func (m *MockBackendClient) WatchServices(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchServicesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchAddressGroups(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchAddressGroupBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupBindingsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchAddressGroupPortMappings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupPortMappingsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchRuleS2S(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchRuleS2SClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchServiceAliases(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchServiceAliasesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchAddressGroupBindingPolicies(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupBindingPoliciesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchHosts(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchHostsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchHostBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchHostBindingsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchNetworks(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchNetworksClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchNetworkBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchNetworkBindingsClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchIEAgAgRules(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchIEAgAgRulesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchSvcSvcRules(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchSvcSvcRulesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}

func (m *MockBackendClient) WatchSvcFqdnRules(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchSvcFqdnRulesClient, error) {
	return nil, fmt.Errorf("watch not implemented in mock backend")
}
