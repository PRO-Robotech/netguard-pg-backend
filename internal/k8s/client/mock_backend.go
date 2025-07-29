package client

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// MockBackendClient с реальными тестовыми данными
type MockBackendClient struct {
	services             []models.Service
	addressGroups        []models.AddressGroup
	addressGroupBindings []models.AddressGroupBinding
	networks             []models.Network
	networkBindings      []models.NetworkBinding
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
	}
}

// Service operations
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

// AddressGroup operations
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

// AddressGroupBinding operations
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

// Stubs for other operations
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

// Network operations
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

// NetworkBinding operations
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

// Ping - простая проверка для mock (всегда успешна)
func (m *MockBackendClient) Ping(ctx context.Context) error {
	return nil
}

// UpdateMeta методы для всех ресурсов (простые заглушки для mock)
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
	return nil // Простая заглушка для mock
}

func (m *MockBackendClient) UpdateRuleS2SMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil // Простая заглушка для mock
}

func (m *MockBackendClient) UpdateServiceAliasMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil // Простая заглушка для mock
}

func (m *MockBackendClient) UpdateAddressGroupBindingPolicyMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error {
	return nil // Простая заглушка для mock
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

// Helper методы для subresources (простые заглушки для mock)
func (m *MockBackendClient) ListAddressGroupsForService(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.AddressGroup, error) {
	// Возвращаем тестовые address groups для mock
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
	// Возвращаем тестовые cross-namespace rules для mock
	return []models.RuleS2S{
		{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.NewResourceIdentifier(
					"test-rule-cross-ns",
					models.WithNamespace("other-namespace"),
				),
			},
			ServiceRef: models.NewServiceAliasRef(serviceID.Name, models.WithNamespace(serviceID.Namespace)),
			Traffic:    "ingress",
		},
	}, nil
}

func (m *MockBackendClient) ListAccessPorts(ctx context.Context, mappingID models.ResourceIdentifier) ([]models.ServicePortsRef, error) {
	// Возвращаем тестовые service ports refs для mock
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

func (m *MockBackendClient) Close() error {
	return nil
}

// Создаем mock клиент вместо реального grpc
func createMockGRPCBackendClient(config BackendClientConfig) (BackendClient, error) {
	return NewMockBackendClient(), nil
}

// Пропускает circuit breaker для mock
func createMockCircuitBreakerClient(client BackendClient, config BackendClientConfig) BackendClient {
	return client
}

// Пропускает cache для mock
func createMockCachedBackendClient(client BackendClient, config BackendClientConfig) BackendClient {
	return client
}
