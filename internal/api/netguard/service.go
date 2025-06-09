package netguard

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	commonpb "netguard-pg-backend/protos/pkg/api/common"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// NetguardServiceServer implements the NetguardService gRPC service
type NetguardServiceServer struct {
	netguardpb.UnimplementedNetguardServiceServer
	service *services.NetguardService
}

// NewNetguardServiceServer creates a new NetguardServiceServer
func NewNetguardServiceServer(service *services.NetguardService) *NetguardServiceServer {
	return &NetguardServiceServer{
		service: service,
	}
}

// Sync syncs data in DB
func (s *NetguardServiceServer) Sync(ctx context.Context, req *netguardpb.SyncReq) (*emptypb.Empty, error) {
	// Convert services
	servicesList := make([]models.Service, 0, len(req.Services))
	for _, svc := range req.Services {
		servicesList = append(servicesList, convertService(svc))
	}

	// Convert address groups
	addressGroups := make([]models.AddressGroup, 0, len(req.AddressGroups))
	for _, ag := range req.AddressGroups {
		addressGroups = append(addressGroups, convertAddressGroup(ag))
	}

	// Convert address group bindings
	bindings := make([]models.AddressGroupBinding, 0, len(req.AddressGroupBindings))
	for _, b := range req.AddressGroupBindings {
		bindings = append(bindings, convertAddressGroupBinding(b))
	}

	// Convert address group port mappings
	mappings := make([]models.AddressGroupPortMapping, 0, len(req.AddressGroupPortMappings))
	for _, m := range req.AddressGroupPortMappings {
		mappings = append(mappings, convertAddressGroupPortMapping(m))
	}

	// Convert rule s2s
	rules := make([]models.RuleS2S, 0, len(req.RuleS2S))
	for _, r := range req.RuleS2S {
		rules = append(rules, convertRuleS2S(r))
	}

	// Sync data
	if err := s.service.SyncServices(ctx, servicesList, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync services")
	}

	if err := s.service.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync address groups")
	}

	if err := s.service.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync address group bindings")
	}

	if err := s.service.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync address group port mappings")
	}

	if err := s.service.SyncRuleS2S(ctx, rules, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync rule s2s")
	}

	return &emptypb.Empty{}, nil
}

// SyncStatus gets last succeeded update DB status
func (s *NetguardServiceServer) SyncStatus(ctx context.Context, _ *emptypb.Empty) (*netguardpb.SyncStatusResp, error) {
	status, err := s.service.GetSyncStatus(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sync status")
	}

	return &netguardpb.SyncStatusResp{
		UpdatedAt: timestamppb.New(status.UpdatedAt),
	}, nil
}

// ListServices gets list of services
func (s *NetguardServiceServer) ListServices(ctx context.Context, req *netguardpb.ListServicesReq) (*netguardpb.ListServicesResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Names) > 0 {
		scope = ports.NewNameScope(req.Names...)
	}

	services, err := s.service.GetServices(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get services")
	}

	items := make([]*netguardpb.Service, 0, len(services))
	for _, svc := range services {
		items = append(items, convertServiceToPB(svc))
	}

	return &netguardpb.ListServicesResp{
		Items: items,
	}, nil
}

// ListAddressGroups gets list of address groups
func (s *NetguardServiceServer) ListAddressGroups(ctx context.Context, req *netguardpb.ListAddressGroupsReq) (*netguardpb.ListAddressGroupsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Names) > 0 {
		scope = ports.NewNameScope(req.Names...)
	}

	addressGroups, err := s.service.GetAddressGroups(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address groups")
	}

	items := make([]*netguardpb.AddressGroup, 0, len(addressGroups))
	for _, ag := range addressGroups {
		items = append(items, convertAddressGroupToPB(ag))
	}

	return &netguardpb.ListAddressGroupsResp{
		Items: items,
	}, nil
}

// ListAddressGroupBindings gets list of address group bindings
func (s *NetguardServiceServer) ListAddressGroupBindings(ctx context.Context, req *netguardpb.ListAddressGroupBindingsReq) (*netguardpb.ListAddressGroupBindingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Names) > 0 {
		scope = ports.NewNameScope(req.Names...)
	}

	bindings, err := s.service.GetAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group bindings")
	}

	items := make([]*netguardpb.AddressGroupBinding, 0, len(bindings))
	for _, b := range bindings {
		items = append(items, convertAddressGroupBindingToPB(b))
	}

	return &netguardpb.ListAddressGroupBindingsResp{
		Items: items,
	}, nil
}

// ListAddressGroupPortMappings gets list of address group port mappings
func (s *NetguardServiceServer) ListAddressGroupPortMappings(ctx context.Context, req *netguardpb.ListAddressGroupPortMappingsReq) (*netguardpb.ListAddressGroupPortMappingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Names) > 0 {
		scope = ports.NewNameScope(req.Names...)
	}

	mappings, err := s.service.GetAddressGroupPortMappings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group port mappings")
	}

	items := make([]*netguardpb.AddressGroupPortMapping, 0, len(mappings))
	for _, m := range mappings {
		items = append(items, convertAddressGroupPortMappingToPB(m))
	}

	return &netguardpb.ListAddressGroupPortMappingsResp{
		Items: items,
	}, nil
}

// ListRuleS2S gets list of rule s2s
func (s *NetguardServiceServer) ListRuleS2S(ctx context.Context, req *netguardpb.ListRuleS2SReq) (*netguardpb.ListRuleS2SResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Names) > 0 {
		scope = ports.NewNameScope(req.Names...)
	}

	rules, err := s.service.GetRuleS2S(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get rule s2s")
	}

	items := make([]*netguardpb.RuleS2S, 0, len(rules))
	for _, r := range rules {
		items = append(items, convertRuleS2SToPB(r))
	}

	return &netguardpb.ListRuleS2SResp{
		Items: items,
	}, nil
}

// Helper functions for converting between protobuf and domain models

func convertService(svc *netguardpb.Service) models.Service {
	result := models.Service{
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		Description: svc.Description,
	}

	// Convert ingress ports
	for _, p := range svc.IngressPorts {
		result.IngressPorts = append(result.IngressPorts, models.IngressPort{
			Protocol:    models.TransportProtocol(p.Protocol.String()),
			Port:        p.Port,
			Description: p.Description,
		})
	}

	// Convert address groups
	for _, ag := range svc.AddressGroups {
		result.AddressGroups = append(result.AddressGroups, models.AddressGroupRef{
			Name:      ag.Name,
			Namespace: ag.Namespace,
		})
	}

	return result
}

func convertAddressGroup(ag *netguardpb.AddressGroup) models.AddressGroup {
	result := models.AddressGroup{
		Name:        ag.Name,
		Namespace:   ag.Namespace,
		Description: ag.Description,
		Addresses:   ag.Addresses,
	}

	// Convert services
	for _, svc := range ag.Services {
		result.Services = append(result.Services, models.ServiceRef{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		})
	}

	return result
}

func convertAddressGroupBinding(b *netguardpb.AddressGroupBinding) models.AddressGroupBinding {
	return models.AddressGroupBinding{
		Name:      b.Name,
		Namespace: b.Namespace,
		ServiceRef: models.ServiceRef{
			Name:      b.ServiceRef.Name,
			Namespace: b.ServiceRef.Namespace,
		},
		AddressGroupRef: models.AddressGroupRef{
			Name:      b.AddressGroupRef.Name,
			Namespace: b.AddressGroupRef.Namespace,
		},
	}
}

func convertAddressGroupPortMapping(m *netguardpb.AddressGroupPortMapping) models.AddressGroupPortMapping {
	result := models.AddressGroupPortMapping{
		Name:      m.Name,
		Namespace: m.Namespace,
	}

	// Convert access ports
	for _, ap := range m.AccessPorts {
		spr := models.ServicePortsRef{
			Name:      ap.Name,
			Namespace: ap.Namespace,
			Ports:     make(models.ProtocolPorts),
		}

		// Convert ports
		for proto, ranges := range ap.Ports.Ports {
			portRanges := make([]models.PortRange, 0, len(ranges.Ranges))
			for _, r := range ranges.Ranges {
				portRanges = append(portRanges, models.PortRange{
					Start: int(r.Start),
					End:   int(r.End),
				})
			}
			spr.Ports[models.TransportProtocol(proto)] = portRanges
		}

		result.AccessPorts = append(result.AccessPorts, spr)
	}

	return result
}

func convertRuleS2S(r *netguardpb.RuleS2S) models.RuleS2S {
	return models.RuleS2S{
		Name:      r.Name,
		Namespace: r.Namespace,
		Traffic:   models.Traffic(r.Traffic.String()),
		ServiceLocalRef: models.ServiceRef{
			Name:      r.ServiceLocalRef.Name,
			Namespace: r.ServiceLocalRef.Namespace,
		},
		ServiceRef: models.ServiceRef{
			Name:      r.ServiceRef.Name,
			Namespace: r.ServiceRef.Namespace,
		},
	}
}

func convertServiceToPB(svc models.Service) *netguardpb.Service {
	result := &netguardpb.Service{
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		Description: svc.Description,
	}

	// Convert ingress ports
	for _, p := range svc.IngressPorts {
		var proto commonpb.Networks_NetIP_Transport
		switch p.Protocol {
		case models.TCP:
			proto = commonpb.Networks_NetIP_TCP
		case models.UDP:
			proto = commonpb.Networks_NetIP_UDP
		}

		result.IngressPorts = append(result.IngressPorts, &netguardpb.IngressPort{
			Protocol:    proto,
			Port:        p.Port,
			Description: p.Description,
		})
	}

	// Convert address groups
	for _, ag := range svc.AddressGroups {
		result.AddressGroups = append(result.AddressGroups, &netguardpb.AddressGroupRef{
			Name:      ag.Name,
			Namespace: ag.Namespace,
		})
	}

	return result
}

func convertAddressGroupToPB(ag models.AddressGroup) *netguardpb.AddressGroup {
	result := &netguardpb.AddressGroup{
		Name:        ag.Name,
		Namespace:   ag.Namespace,
		Description: ag.Description,
		Addresses:   ag.Addresses,
	}

	// Convert services
	for _, svc := range ag.Services {
		result.Services = append(result.Services, &netguardpb.ServiceRef{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		})
	}

	return result
}

func convertAddressGroupBindingToPB(b models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	return &netguardpb.AddressGroupBinding{
		Name:      b.Name,
		Namespace: b.Namespace,
		ServiceRef: &netguardpb.ServiceRef{
			Name:      b.ServiceRef.Name,
			Namespace: b.ServiceRef.Namespace,
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Name:      b.AddressGroupRef.Name,
			Namespace: b.AddressGroupRef.Namespace,
		},
	}
}

func convertAddressGroupPortMappingToPB(m models.AddressGroupPortMapping) *netguardpb.AddressGroupPortMapping {
	result := &netguardpb.AddressGroupPortMapping{
		Name:      m.Name,
		Namespace: m.Namespace,
	}

	// Convert access ports
	for _, ap := range m.AccessPorts {
		spr := &netguardpb.ServicePortsRef{
			Name:      ap.Name,
			Namespace: ap.Namespace,
			Ports: &netguardpb.ProtocolPorts{
				Ports: make(map[string]*netguardpb.PortRanges),
			},
		}

		// Convert ports
		for proto, ranges := range ap.Ports {
			portRanges := make([]*netguardpb.PortRange, 0, len(ranges))
			for _, r := range ranges {
				portRanges = append(portRanges, &netguardpb.PortRange{
					Start: int32(r.Start),
					End:   int32(r.End),
				})
			}
			spr.Ports.Ports[string(proto)] = &netguardpb.PortRanges{
				Ranges: portRanges,
			}
		}

		result.AccessPorts = append(result.AccessPorts, spr)
	}

	return result
}

func convertRuleS2SToPB(r models.RuleS2S) *netguardpb.RuleS2S {
	var traffic commonpb.Traffic
	switch r.Traffic {
	case models.INGRESS:
		traffic = commonpb.Traffic_Ingress
	case models.EGRESS:
		traffic = commonpb.Traffic_Egress
	}

	return &netguardpb.RuleS2S{
		Name:      r.Name,
		Namespace: r.Namespace,
		Traffic:   traffic,
		ServiceLocalRef: &netguardpb.ServiceRef{
			Name:      r.ServiceLocalRef.Name,
			Namespace: r.ServiceLocalRef.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Name:      r.ServiceRef.Name,
			Namespace: r.ServiceRef.Namespace,
		},
	}
}
