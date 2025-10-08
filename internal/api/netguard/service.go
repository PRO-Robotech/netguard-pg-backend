package netguard

import (
	"context"

	"netguard-pg-backend/internal/api/netguard/handlers"
	"netguard-pg-backend/internal/api/netguard/sync"
	"netguard-pg-backend/internal/application/services"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ServiceServer implements the NetguardService gRPC interface
type ServiceServer struct {
	netguardpb.UnimplementedNetguardServiceServer

	serviceHandler      *handlers.ServiceHandler
	addressGroupHandler *handlers.AddressGroupHandler
	ruleHandler         *handlers.RuleHandler
	networkHandler      *handlers.NetworkHandler
	hostHandler         *handlers.HostHandler

	syncDispatcher *sync.Dispatcher

	service *services.NetguardFacade
}

// NewServiceServer creates a new NetguardServiceServer
func NewServiceServer(service *services.NetguardFacade) *ServiceServer {
	return &ServiceServer{
		serviceHandler:      handlers.NewServiceHandler(service),
		addressGroupHandler: handlers.NewAddressGroupHandler(service),
		ruleHandler:         handlers.NewRuleHandler(service),
		networkHandler:      handlers.NewNetworkHandler(service),
		hostHandler:         handlers.NewHostHandler(service),
		syncDispatcher:      sync.NewDispatcher(service),
		service:             service,
	}
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

func (s *ServiceServer) ListServiceAliases(ctx context.Context, req *netguardpb.ListServiceAliasesReq) (*netguardpb.ListServiceAliasesResp, error) {
	return s.serviceHandler.ListServiceAliases(ctx, req)
}

func (s *ServiceServer) GetServiceAlias(ctx context.Context, req *netguardpb.GetServiceAliasReq) (*netguardpb.GetServiceAliasResp, error) {
	return s.serviceHandler.GetServiceAlias(ctx, req)
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

func (s *ServiceServer) ListRuleS2S(ctx context.Context, req *netguardpb.ListRuleS2SReq) (*netguardpb.ListRuleS2SResp, error) {
	return s.ruleHandler.ListRuleS2S(ctx, req)
}

func (s *ServiceServer) GetRuleS2S(ctx context.Context, req *netguardpb.GetRuleS2SReq) (*netguardpb.GetRuleS2SResp, error) {
	return s.ruleHandler.GetRuleS2S(ctx, req)
}

func (s *ServiceServer) ListIEAgAgRules(ctx context.Context, req *netguardpb.ListIEAgAgRulesReq) (*netguardpb.ListIEAgAgRulesResp, error) {
	return s.ruleHandler.ListIEAgAgRules(ctx, req)
}

func (s *ServiceServer) GetIEAgAgRule(ctx context.Context, req *netguardpb.GetIEAgAgRuleReq) (*netguardpb.GetIEAgAgRuleResp, error) {
	return s.ruleHandler.GetIEAgAgRule(ctx, req)
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
