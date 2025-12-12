package client

import (
	"context"
	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"k8s.io/klog/v2"
)

type BackendClient interface {
	GetService(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error)
	ListServices(ctx context.Context, scope ports.Scope) ([]models.Service, error)
	CreateService(ctx context.Context, service *models.Service) error
	UpdateService(ctx context.Context, service *models.Service) error
	DeleteService(ctx context.Context, id models.ResourceIdentifier) error
	GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error)
	ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error)
	CreateAddressGroup(ctx context.Context, group *models.AddressGroup) error
	UpdateAddressGroup(ctx context.Context, group *models.AddressGroup) error
	DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error
	GetAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error)
	ListAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error)
	CreateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error
	UpdateAddressGroupBinding(ctx context.Context, binding *models.AddressGroupBinding) error
	DeleteAddressGroupBinding(ctx context.Context, id models.ResourceIdentifier) error
	GetAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error)
	ListAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error)
	CreateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	UpdateAddressGroupPortMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) error
	DeleteAddressGroupPortMapping(ctx context.Context, id models.ResourceIdentifier) error
	GetAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error)
	ListAddressGroupBindingPolicies(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBindingPolicy, error)
	CreateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error
	UpdateAddressGroupBindingPolicy(ctx context.Context, policy *models.AddressGroupBindingPolicy) error
	DeleteAddressGroupBindingPolicy(ctx context.Context, id models.ResourceIdentifier) error
	GetNetwork(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error)
	ListNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error)
	CreateNetwork(ctx context.Context, network *models.Network) error
	UpdateNetwork(ctx context.Context, network *models.Network) error
	DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error
	GetNetworkBinding(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error)
	ListNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error)
	CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error
	UpdateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error
	DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error
	GetHost(ctx context.Context, id models.ResourceIdentifier) (*models.Host, error)
	ListHosts(ctx context.Context, scope ports.Scope) ([]models.Host, error)
	CreateHost(ctx context.Context, host *models.Host) error
	UpdateHost(ctx context.Context, host *models.Host) error
	DeleteHost(ctx context.Context, id models.ResourceIdentifier) error
	GetHostBinding(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error)
	ListHostBindings(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error)
	CreateHostBinding(ctx context.Context, binding *models.HostBinding) error
	UpdateHostBinding(ctx context.Context, binding *models.HostBinding) error
	DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error
	GetSvcSvcRule(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error)
	ListSvcSvcRules(ctx context.Context, scope ports.Scope) ([]models.SvcSvcRule, error)
	CreateSvcSvcRule(ctx context.Context, rule *models.SvcSvcRule) error
	UpdateSvcSvcRule(ctx context.Context, rule *models.SvcSvcRule) error
	DeleteSvcSvcRule(ctx context.Context, id models.ResourceIdentifier) error
	GetSvcFqdnRule(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error)
	ListSvcFqdnRules(ctx context.Context, scope ports.Scope) ([]models.SvcFqdnRule, error)
	CreateSvcFqdnRule(ctx context.Context, rule *models.SvcFqdnRule) error
	UpdateSvcFqdnRule(ctx context.Context, rule *models.SvcFqdnRule) error
	DeleteSvcFqdnRule(ctx context.Context, id models.ResourceIdentifier) error
	Sync(ctx context.Context, syncOp models.SyncOp, resources interface{}) error
	GetSyncStatus(ctx context.Context) (*models.SyncStatus, error)
	GetDependencyValidator() *validation.DependencyValidator
	GetReader(ctx context.Context) (ports.Reader, error)
	HealthCheck(ctx context.Context) error
	Ping(ctx context.Context) error
	UpdateServiceMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateAddressGroupMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateAddressGroupBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateAddressGroupPortMappingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateAddressGroupBindingPolicyMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateNetworkMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateNetworkBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateHostMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateHostBindingMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateSvcSvcRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	UpdateSvcFqdnRuleMeta(ctx context.Context, id models.ResourceIdentifier, meta models.Meta) error
	ListAddressGroupsForService(ctx context.Context, serviceID models.ResourceIdentifier) ([]models.AddressGroup, error)
	ListAccessPorts(ctx context.Context, mappingID models.ResourceIdentifier) ([]models.ServicePortsRef, error)
	MarkForDeletion(ctx context.Context, namespace, name, kind string) error
	WatchServices(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchServicesClient, error)
	WatchAddressGroups(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupsClient, error)
	WatchAddressGroupBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupBindingsClient, error)
	WatchAddressGroupPortMappings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupPortMappingsClient, error)
	WatchAddressGroupBindingPolicies(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchAddressGroupBindingPoliciesClient, error)
	WatchHosts(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchHostsClient, error)
	WatchHostBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchHostBindingsClient, error)
	WatchNetworks(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchNetworksClient, error)
	WatchNetworkBindings(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchNetworkBindingsClient, error)
	WatchSvcSvcRules(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchSvcSvcRulesClient, error)
	WatchSvcFqdnRules(ctx context.Context, req *netguardpb.WatchRequest) (netguardpb.NetguardService_WatchSvcFqdnRulesClient, error)
	Close() error
}

func NewBackendClient(config BackendClientConfig) (BackendClient, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Endpoint == "mock" {
		return NewMockBackendClient(), nil
	}
	klog.Infof("Dialing backend gRPC %s …", config.Endpoint)
	grpcClient, err := NewGRPCBackendClient(config)
	if err != nil {
		klog.Errorf("backend connection failed: %v", err)
		return nil, err
	}
	klog.Infof("Connected to backend gRPC %s", config.Endpoint)
	return grpcClient, nil
}
