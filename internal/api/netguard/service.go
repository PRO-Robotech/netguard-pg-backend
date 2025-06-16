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

func (s *NetguardServiceServer) Sync(ctx context.Context, req *netguardpb.SyncReq) (*emptypb.Empty, error) {
	// Convert services
	servicesList := make([]models.Service, 0, len(req.Services))
	for _, svc := range req.Services {
		servicesList = append(servicesList, convertService(svc))
	}

	// Convert service aliases
	serviceAliasesList := make([]models.ServiceAlias, 0, len(req.ServiceAliases))
	for _, svcAlias := range req.ServiceAliases {
		serviceAliasesList = append(serviceAliasesList, convertServiceAlias(svcAlias))
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

	if err := s.service.SyncServiceAliases(ctx, serviceAliasesList, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to sync service aliases")
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
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
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

func idFromReq(ri *netguardpb.ResourceIdentifier) models.ResourceIdentifier {
	return models.NewResourceIdentifier(ri.GetName(), models.WithNamespace(ri.GetNamespace()))
}

// GetService gets a specific service by ID
func (s *NetguardServiceServer) GetService(ctx context.Context, req *netguardpb.GetServiceReq) (*netguardpb.GetServiceResp, error) {
	id := idFromReq(req.GetIdentifier())
	service, err := s.service.GetServiceByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service")
	}

	return &netguardpb.GetServiceResp{
		Service: convertServiceToPB(*service),
	}, nil
}

// ListAddressGroups gets list of address groups
func (s *NetguardServiceServer) ListAddressGroups(ctx context.Context, req *netguardpb.ListAddressGroupsReq) (*netguardpb.ListAddressGroupsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.GetIdentifiers()) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.GetIdentifiers()))
		for _, id := range req.GetIdentifiers() {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.GetName(), models.WithNamespace(id.GetNamespace())))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
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

// GetAddressGroup gets a specific address group by ID
func (s *NetguardServiceServer) GetAddressGroup(ctx context.Context, req *netguardpb.GetAddressGroupReq) (*netguardpb.GetAddressGroupResp, error) {
	id := idFromReq(req.GetIdentifier())
	addressGroup, err := s.service.GetAddressGroupByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group")
	}

	return &netguardpb.GetAddressGroupResp{
		AddressGroup: convertAddressGroupToPB(*addressGroup),
	}, nil
}

// ListAddressGroupBindings gets list of address group bindings
func (s *NetguardServiceServer) ListAddressGroupBindings(ctx context.Context, req *netguardpb.ListAddressGroupBindingsReq) (*netguardpb.ListAddressGroupBindingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
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

// GetAddressGroupBinding gets a specific address group binding by ID
func (s *NetguardServiceServer) GetAddressGroupBinding(ctx context.Context, req *netguardpb.GetAddressGroupBindingReq) (*netguardpb.GetAddressGroupBindingResp, error) {
	id := idFromReq(req.GetIdentifier())
	binding, err := s.service.GetAddressGroupBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding")
	}

	return &netguardpb.GetAddressGroupBindingResp{
		AddressGroupBinding: convertAddressGroupBindingToPB(*binding),
	}, nil
}

// ListAddressGroupPortMappings gets list of address group port mappings
func (s *NetguardServiceServer) ListAddressGroupPortMappings(ctx context.Context, req *netguardpb.ListAddressGroupPortMappingsReq) (*netguardpb.ListAddressGroupPortMappingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
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

// GetAddressGroupPortMapping gets a specific address group port mapping by ID
func (s *NetguardServiceServer) GetAddressGroupPortMapping(ctx context.Context, req *netguardpb.GetAddressGroupPortMappingReq) (*netguardpb.GetAddressGroupPortMappingResp, error) {
	id := idFromReq(req.GetIdentifier())
	mapping, err := s.service.GetAddressGroupPortMappingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group port mapping")
	}

	return &netguardpb.GetAddressGroupPortMappingResp{
		AddressGroupPortMapping: convertAddressGroupPortMappingToPB(*mapping),
	}, nil
}

// ListRuleS2S gets list of rule s2s
func (s *NetguardServiceServer) ListRuleS2S(ctx context.Context, req *netguardpb.ListRuleS2SReq) (*netguardpb.ListRuleS2SResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
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

// GetRuleS2S gets a specific rule s2s by ID
func (s *NetguardServiceServer) GetRuleS2S(ctx context.Context, req *netguardpb.GetRuleS2SReq) (*netguardpb.GetRuleS2SResp, error) {
	id := idFromReq(req.GetIdentifier())
	rule, err := s.service.GetRuleS2SByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get rule s2s")
	}

	return &netguardpb.GetRuleS2SResp{
		RuleS2S: convertRuleS2SToPB(*rule),
	}, nil
}

// ListServiceAliases gets list of service aliases
func (s *NetguardServiceServer) ListServiceAliases(ctx context.Context, req *netguardpb.ListServiceAliasesReq) (*netguardpb.ListServiceAliasesResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	aliases, err := s.service.GetServiceAliases(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service aliases")
	}

	items := make([]*netguardpb.ServiceAlias, 0, len(aliases))
	for _, a := range aliases {
		items = append(items, convertServiceAliasToPB(a))
	}

	return &netguardpb.ListServiceAliasesResp{
		Items: items,
	}, nil
}

// GetServiceAlias gets a specific service alias by ID
func (s *NetguardServiceServer) GetServiceAlias(ctx context.Context, req *netguardpb.GetServiceAliasReq) (*netguardpb.GetServiceAliasResp, error) {
	id := idFromReq(req.GetIdentifier())
	alias, err := s.service.GetServiceAliasByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service alias")
	}

	return &netguardpb.GetServiceAliasResp{
		ServiceAlias: convertServiceAliasToPB(*alias),
	}, nil
}

// Helper functions for converting between protobuf and domain models

func getSelfRef(identifier *netguardpb.ResourceIdentifier) models.ResourceIdentifier {
	return models.NewResourceIdentifier(identifier.GetName(), models.WithNamespace(identifier.GetNamespace()))
}

func convertService(svc *netguardpb.Service) models.Service {
	result := models.Service{
		SelfRef:     models.NewSelfRef(getSelfRef(svc.GetSelfRef())),
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
		ref := models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(ag.GetIdentifier().GetName(),
				models.WithNamespace(ag.GetIdentifier().GetNamespace())),
		}
		result.AddressGroups = append(result.AddressGroups, ref)
	}

	return result
}

func convertAddressGroup(ag *netguardpb.AddressGroup) models.AddressGroup {
	result := models.AddressGroup{
		SelfRef:     models.NewSelfRef(getSelfRef(ag.GetSelfRef())),
		Description: ag.Description,
		Addresses:   ag.Addresses,
	}

	// Convert services
	for _, svc := range ag.Services {
		ref := models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(svc.GetIdentifier().GetName(),
				models.WithNamespace(svc.GetIdentifier().GetNamespace())),
		}
		result.Services = append(result.Services, ref)
	}

	return result
}

func convertAddressGroupBinding(b *netguardpb.AddressGroupBinding) models.AddressGroupBinding {
	result := models.AddressGroupBinding{
		SelfRef: models.NewSelfRef(getSelfRef(b.GetSelfRef())),
	}

	// Convert ServiceRef
	result.ServiceRef = models.NewServiceRef(b.GetServiceRef().GetIdentifier().GetName(),
		models.WithNamespace(b.GetServiceRef().GetIdentifier().GetNamespace()))

	// Convert AddressGroupRef
	result.AddressGroupRef = models.NewAddressGroupRef(b.GetAddressGroupRef().GetIdentifier().GetName(),
		models.WithNamespace(b.GetAddressGroupRef().GetIdentifier().GetNamespace()))

	return result
}

func convertAddressGroupPortMapping(m *netguardpb.AddressGroupPortMapping) models.AddressGroupPortMapping {
	result := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(getSelfRef(m.GetSelfRef())),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{},
	}

	// Convert access ports
	for _, ap := range m.AccessPorts {
		spr := models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(ap.Identifier.Name, models.WithNamespace(ap.GetIdentifier().GetNamespace())),
		}
		ports := make(models.ProtocolPorts)

		// Convert ports
		for proto, ranges := range ap.Ports.Ports {
			portRanges := make([]models.PortRange, 0, len(ranges.Ranges))
			for _, r := range ranges.Ranges {
				portRanges = append(portRanges, models.PortRange{
					Start: int(r.Start),
					End:   int(r.End),
				})
			}
			ports[models.TransportProtocol(proto)] = portRanges
		}

		result.AccessPorts[spr] = models.ServicePorts{Ports: ports}
	}

	return result
}

func convertRuleS2S(r *netguardpb.RuleS2S) models.RuleS2S {
	result := models.RuleS2S{
		SelfRef: models.NewSelfRef(getSelfRef(r.GetSelfRef())),
		Traffic: models.Traffic(r.Traffic.String()),
	}

	// Convert ServiceLocalRef
	result.ServiceLocalRef = models.ServiceAliasRef{
		ResourceIdentifier: models.NewResourceIdentifier(r.ServiceLocalRef.Identifier.Name, models.WithNamespace(r.GetServiceLocalRef().GetIdentifier().GetNamespace())),
	}

	// Convert ServiceRef
	result.ServiceRef = models.ServiceAliasRef{
		ResourceIdentifier: models.NewResourceIdentifier(r.ServiceRef.Identifier.Name, models.WithNamespace(r.GetServiceRef().GetIdentifier().GetNamespace())),
	}

	return result
}

func convertServiceToPB(svc models.Service) *netguardpb.Service {
	result := &netguardpb.Service{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      svc.ResourceIdentifier.Name,
			Namespace: svc.ResourceIdentifier.Namespace,
		},
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
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      ag.ResourceIdentifier.Name,
				Namespace: ag.ResourceIdentifier.Namespace,
			},
		})
	}

	return result
}

func convertAddressGroupToPB(ag models.AddressGroup) *netguardpb.AddressGroup {
	result := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      ag.ResourceIdentifier.Name,
			Namespace: ag.ResourceIdentifier.Namespace,
		},
		Description: ag.Description,
		Addresses:   ag.Addresses,
	}

	// Convert services
	for _, svc := range ag.Services {
		result.Services = append(result.Services, &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      svc.ResourceIdentifier.Name,
				Namespace: svc.ResourceIdentifier.Namespace,
			},
		})
	}

	return result
}

func convertAddressGroupBindingToPB(b models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	return &netguardpb.AddressGroupBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      b.ResourceIdentifier.Name,
			Namespace: b.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      b.ServiceRef.ResourceIdentifier.Name,
				Namespace: b.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      b.AddressGroupRef.ResourceIdentifier.Name,
				Namespace: b.AddressGroupRef.ResourceIdentifier.Namespace,
			},
		},
	}
}

func convertAddressGroupPortMappingToPB(m models.AddressGroupPortMapping) *netguardpb.AddressGroupPortMapping {
	result := &netguardpb.AddressGroupPortMapping{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
	}

	// Convert access ports
	for srv, ap := range m.AccessPorts {
		spr := &netguardpb.ServicePortsRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      srv.ResourceIdentifier.Name,
				Namespace: srv.ResourceIdentifier.Namespace,
			},
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
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      r.ResourceIdentifier.Name,
			Namespace: r.ResourceIdentifier.Namespace,
		},
		Traffic: traffic,
		ServiceLocalRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      r.ServiceLocalRef.ResourceIdentifier.Name,
				Namespace: r.ServiceLocalRef.ResourceIdentifier.Namespace,
			},
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      r.ServiceRef.ResourceIdentifier.Name,
				Namespace: r.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
	}
}

func convertServiceAliasToPB(a models.ServiceAlias) *netguardpb.ServiceAlias {
	return &netguardpb.ServiceAlias{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      a.ResourceIdentifier.Name,
			Namespace: a.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      a.ServiceRef.ResourceIdentifier.Name,
				Namespace: a.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
	}
}

func convertServiceAlias(a *netguardpb.ServiceAlias) models.ServiceAlias {
	return models.ServiceAlias{
		SelfRef: models.NewSelfRef(getSelfRef(a.GetSelfRef())),
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(a.GetServiceRef().GetIdentifier().GetName(), models.WithNamespace(a.GetServiceRef().GetIdentifier().GetNamespace())),
		},
	}
}
