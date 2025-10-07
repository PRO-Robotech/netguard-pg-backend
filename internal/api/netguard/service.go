package netguard

import (
	"context"

	"netguard-pg-backend/internal/api/netguard/handlers"
	"netguard-pg-backend/internal/api/netguard/sync"
	"netguard-pg-backend/internal/application/services"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/emptypb"
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

func (s *ServiceServer) ListAddressGroups(ctx context.Context, req *netguardpb.ListAddressGroupsReq) (*netguardpb.ListAddressGroupsResp, error) {
	return s.addressGroupHandler.ListAddressGroups(ctx, req)
}

func (s *ServiceServer) GetAddressGroup(ctx context.Context, req *netguardpb.GetAddressGroupReq) (*netguardpb.GetAddressGroupResp, error) {
	return s.addressGroupHandler.GetAddressGroup(ctx, req)
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

func (s *ServiceServer) ListHosts(ctx context.Context, req *netguardpb.ListHostsReq) (*netguardpb.ListHostsResp, error) {
	return s.hostHandler.ListHosts(ctx, req)
}

func (s *ServiceServer) GetHost(ctx context.Context, req *netguardpb.GetHostReq) (*netguardpb.GetHostResp, error) {
	return s.hostHandler.GetHost(ctx, req)
}
