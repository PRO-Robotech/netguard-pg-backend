package netguard

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/api/netguard/handlers"
	"netguard-pg-backend/internal/api/netguard/sync"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/config"
	watchpkg "netguard-pg-backend/internal/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type watchCacheProvider interface {
	Cache(resourceType string) (*watchpkg.WatchCache, bool)
}

type ServiceServer struct {
	netguardpb.UnimplementedNetguardServiceServer
	serviceHandler       *handlers.ServiceHandler
	addressGroupHandler  *handlers.AddressGroupHandler
	svcSvcRuleHandler    *handlers.SvcSvcRuleHandler
	svcFqdnRuleHandler   *handlers.SvcFqdnRuleHandler
	ieCidrSvcRuleHandler *handlers.IECidrSvcRuleHandler
	networkHandler       *handlers.NetworkHandler
	hostHandler          *handlers.HostHandler
	syncDispatcher       *sync.Dispatcher
	service              *services.NetguardFacade
	watchHandler         *WatchHandler
	watchConfig          config.WatchConfig
	watchProvider        watchCacheProvider
}

func NewServiceServer(service *services.NetguardFacade, watchCfg config.WatchConfig) *ServiceServer {
	return &ServiceServer{
		serviceHandler:       handlers.NewServiceHandler(service),
		addressGroupHandler:  handlers.NewAddressGroupHandler(service),
		svcSvcRuleHandler:    handlers.NewSvcSvcRuleHandler(service),
		svcFqdnRuleHandler:   handlers.NewSvcFqdnRuleHandler(service),
		ieCidrSvcRuleHandler: handlers.NewIECidrSvcRuleHandler(service),
		networkHandler:       handlers.NewNetworkHandler(service),
		hostHandler:          handlers.NewHostHandler(service),
		syncDispatcher:       sync.NewDispatcher(service),
		service:              service,
		watchConfig:          watchCfg,
	}
}

func (s *ServiceServer) SetWatchManager(provider watchCacheProvider) {
	s.watchProvider = provider
	if provider != nil {
		s.watchHandler = NewWatchHandler(provider, s.watchConfig)
	}
}

func (s *ServiceServer) streamWatch(resourceType string, req *netguardpb.WatchRequest, ctx context.Context, send func(*netguardpb.WatchEvent) error) error {
	if s.watchHandler == nil {
		return status.Error(codes.Unimplemented, "watch manager not configured")
	}
	return s.watchHandler.Stream(ctx, resourceType, req, send)
}
func (s *ServiceServer) Sync(ctx context.Context, req *netguardpb.SyncReq) (*emptypb.Empty, error) {
	return s.syncDispatcher.Sync(ctx, req)
}
func (s *ServiceServer) ListServices(ctx context.Context, req *netguardpb.ListServicesReq) (*netguardpb.ListServicesResp, error) {
	return s.serviceHandler.ListServices(ctx, req)
}
func (s *ServiceServer) GetService(ctx context.Context, req *netguardpb.GetServiceReq) (*netguardpb.GetServiceResp, error) {
	return s.serviceHandler.GetService(ctx, req)
}
func (s *ServiceServer) ListAddressGroups(ctx context.Context, req *netguardpb.ListAddressGroupsReq) (*netguardpb.ListAddressGroupsResp, error) {
	return s.addressGroupHandler.ListAddressGroups(ctx, req)
}
func (s *ServiceServer) GetAddressGroup(ctx context.Context, req *netguardpb.GetAddressGroupReq) (*netguardpb.GetAddressGroupResp, error) {
	return s.addressGroupHandler.GetAddressGroup(ctx, req)
}
func (s *ServiceServer) ListAddressGroupBindings(ctx context.Context, req *netguardpb.ListAddressGroupBindingsReq) (*netguardpb.ListAddressGroupBindingsResp, error) {
	return s.addressGroupHandler.ListAddressGroupBindings(ctx, req)
}
func (s *ServiceServer) GetAddressGroupBinding(ctx context.Context, req *netguardpb.GetAddressGroupBindingReq) (*netguardpb.GetAddressGroupBindingResp, error) {
	return s.addressGroupHandler.GetAddressGroupBinding(ctx, req)
}
func (s *ServiceServer) ListAddressGroupPortMappings(ctx context.Context, req *netguardpb.ListAddressGroupPortMappingsReq) (*netguardpb.ListAddressGroupPortMappingsResp, error) {
	return s.addressGroupHandler.ListAddressGroupPortMappings(ctx, req)
}
func (s *ServiceServer) GetAddressGroupPortMapping(ctx context.Context, req *netguardpb.GetAddressGroupPortMappingReq) (*netguardpb.GetAddressGroupPortMappingResp, error) {
	return s.addressGroupHandler.GetAddressGroupPortMapping(ctx, req)
}
func (s *ServiceServer) ListAddressGroupBindingPolicies(ctx context.Context, req *netguardpb.ListAddressGroupBindingPoliciesReq) (*netguardpb.ListAddressGroupBindingPoliciesResp, error) {
	return s.addressGroupHandler.ListAddressGroupBindingPolicies(ctx, req)
}
func (s *ServiceServer) GetAddressGroupBindingPolicy(ctx context.Context, req *netguardpb.GetAddressGroupBindingPolicyReq) (*netguardpb.GetAddressGroupBindingPolicyResp, error) {
	return s.addressGroupHandler.GetAddressGroupBindingPolicy(ctx, req)
}
func (s *ServiceServer) ListSvcSvcRules(ctx context.Context, req *netguardpb.ListSvcSvcRulesReq) (*netguardpb.ListSvcSvcRulesResp, error) {
	return s.svcSvcRuleHandler.ListSvcSvcRules(ctx, req)
}
func (s *ServiceServer) GetSvcSvcRule(ctx context.Context, req *netguardpb.GetSvcSvcRuleReq) (*netguardpb.GetSvcSvcRuleResp, error) {
	return s.svcSvcRuleHandler.GetSvcSvcRule(ctx, req)
}
func (s *ServiceServer) ListSvcFqdnRules(ctx context.Context, req *netguardpb.ListSvcFqdnRulesReq) (*netguardpb.ListSvcFqdnRulesResp, error) {
	return s.svcFqdnRuleHandler.ListSvcFqdnRules(ctx, req)
}
func (s *ServiceServer) GetSvcFqdnRule(ctx context.Context, req *netguardpb.GetSvcFqdnRuleReq) (*netguardpb.GetSvcFqdnRuleResp, error) {
	return s.svcFqdnRuleHandler.GetSvcFqdnRule(ctx, req)
}
func (s *ServiceServer) ListIECidrSvcRules(ctx context.Context, req *netguardpb.ListIECidrSvcRulesReq) (*netguardpb.ListIECidrSvcRulesResp, error) {
	return s.ieCidrSvcRuleHandler.ListIECidrSvcRules(ctx, req)
}
func (s *ServiceServer) GetIECidrSvcRule(ctx context.Context, req *netguardpb.GetIECidrSvcRuleReq) (*netguardpb.GetIECidrSvcRuleResp, error) {
	return s.ieCidrSvcRuleHandler.GetIECidrSvcRule(ctx, req)
}

// Watch RPC implementations

func (s *ServiceServer) WatchServices(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchServicesServer) error {
	return s.streamWatch("services", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchAddressGroups(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchAddressGroupsServer) error {
	return s.streamWatch("address_groups", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchAddressGroupBindings(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchAddressGroupBindingsServer) error {
	return s.streamWatch("address_group_bindings", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchAddressGroupPortMappings(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchAddressGroupPortMappingsServer) error {
	return s.streamWatch("address_group_port_mappings", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchAddressGroupBindingPolicies(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchAddressGroupBindingPoliciesServer) error {
	return s.streamWatch("address_group_binding_policies", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchHosts(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchHostsServer) error {
	return s.streamWatch("hosts", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchHostBindings(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchHostBindingsServer) error {
	return s.streamWatch("host_bindings", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchNetworks(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchNetworksServer) error {
	return s.streamWatch("networks", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchNetworkBindings(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchNetworkBindingsServer) error {
	return s.streamWatch("network_bindings", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchSvcSvcRules(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchSvcSvcRulesServer) error {
	return s.streamWatch("svc_svc_rules", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchSvcFqdnRules(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchSvcFqdnRulesServer) error {
	return s.streamWatch("svc_fqdn_rules", req, stream.Context(), stream.Send)
}

func (s *ServiceServer) WatchIECidrSvcRules(req *netguardpb.WatchRequest, stream netguardpb.NetguardService_WatchIECidrSvcRulesServer) error {
	return s.streamWatch("ie_cidr_svc_rules", req, stream.Context(), stream.Send)
}
func (s *ServiceServer) ListNetworks(ctx context.Context, req *netguardpb.ListNetworksReq) (*netguardpb.ListNetworksResp, error) {
	return s.networkHandler.ListNetworks(ctx, req)
}
func (s *ServiceServer) GetNetwork(ctx context.Context, req *netguardpb.GetNetworkReq) (*netguardpb.GetNetworkResp, error) {
	return s.networkHandler.GetNetwork(ctx, req)
}
func (s *ServiceServer) ListNetworkBindings(ctx context.Context, req *netguardpb.ListNetworkBindingsReq) (*netguardpb.ListNetworkBindingsResp, error) {
	return s.networkHandler.ListNetworkBindings(ctx, req)
}
func (s *ServiceServer) GetNetworkBinding(ctx context.Context, req *netguardpb.GetNetworkBindingReq) (*netguardpb.GetNetworkBindingResp, error) {
	return s.networkHandler.GetNetworkBinding(ctx, req)
}
func (s *ServiceServer) ListHosts(ctx context.Context, req *netguardpb.ListHostsReq) (*netguardpb.ListHostsResp, error) {
	return s.hostHandler.ListHosts(ctx, req)
}
func (s *ServiceServer) ListHostBindings(ctx context.Context, req *netguardpb.ListHostBindingsReq) (*netguardpb.ListHostBindingsResp, error) {
	return s.hostHandler.ListHostBindings(ctx, req)
}
func (s *ServiceServer) GetHost(ctx context.Context, req *netguardpb.GetHostReq) (*netguardpb.GetHostResp, error) {
	return s.hostHandler.GetHost(ctx, req)
}
func (s *ServiceServer) GetHostBinding(ctx context.Context, req *netguardpb.GetHostBindingReq) (*netguardpb.GetHostBindingResp, error) {
	return s.hostHandler.GetHostBinding(ctx, req)
}
func (s *ServiceServer) SyncStatus(ctx context.Context, _ *emptypb.Empty) (*netguardpb.SyncStatusResp, error) {
	status, err := s.service.GetSyncStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &netguardpb.SyncStatusResp{
		UpdatedAt: timestamppb.New(status.UpdatedAt),
	}, nil
}
func (s *ServiceServer) MarkForDeletion(ctx context.Context, req *netguardpb.MarkForDeletionReq) (*emptypb.Empty, error) {
	err := s.service.MarkForDeletion(ctx, req.Namespace, req.Name, req.Kind)
	if err != nil {
		klog.ErrorS(err, "NetguardFacade.MarkForDeletion failed",
			"namespace", req.Namespace,
			"name", req.Name,
			"kind", req.Kind)
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
