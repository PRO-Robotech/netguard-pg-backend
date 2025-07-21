package netguard

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// convertSyncOp преобразует proto SyncOp в models.SyncOp
func convertSyncOp(protoSyncOp netguardpb.SyncOp) models.SyncOp {
	return models.ProtoToSyncOp(int32(protoSyncOp))
}

// convertSyncOpToPB преобразует models.SyncOp в proto SyncOp
func convertSyncOpToPB(syncOp models.SyncOp) netguardpb.SyncOp {
	return netguardpb.SyncOp(models.SyncOpToProto(syncOp))
}

func (s *NetguardServiceServer) Sync(ctx context.Context, req *netguardpb.SyncReq) (*emptypb.Empty, error) {
	// Преобразуем тип операции из proto в модель
	syncOp := convertSyncOp(req.SyncOp)

	// Обрабатываем разные типы субъектов
	var err error
	switch subject := req.Subject.(type) {
	case *netguardpb.SyncReq_Services:
		if subject.Services == nil || len(subject.Services.Services) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем сервисы
		services := make([]models.Service, 0, len(subject.Services.Services))
		for _, svc := range subject.Services.Services {
			services = append(services, convertService(svc))
		}

		// Синхронизируем сервисы с указанной операцией
		err = s.service.Sync(ctx, syncOp, services)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync services")
		}

	case *netguardpb.SyncReq_AddressGroups:
		if subject.AddressGroups == nil || len(subject.AddressGroups.AddressGroups) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем группы адресов
		addressGroups := make([]models.AddressGroup, 0, len(subject.AddressGroups.AddressGroups))
		for _, ag := range subject.AddressGroups.AddressGroups {
			addressGroups = append(addressGroups, convertAddressGroup(ag))
		}

		// Синхронизируем группы адресов с указанной операцией
		err = s.service.Sync(ctx, syncOp, addressGroups)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync address groups")
		}

	case *netguardpb.SyncReq_AddressGroupBindings:
		if subject.AddressGroupBindings == nil || len(subject.AddressGroupBindings.AddressGroupBindings) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем привязки групп адресов
		bindings := make([]models.AddressGroupBinding, 0, len(subject.AddressGroupBindings.AddressGroupBindings))
		for _, b := range subject.AddressGroupBindings.AddressGroupBindings {
			bindings = append(bindings, convertAddressGroupBinding(b))
		}

		// Синхронизируем привязки групп адресов с указанной операцией
		err = s.service.Sync(ctx, syncOp, bindings)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync address group bindings")
		}

	case *netguardpb.SyncReq_AddressGroupPortMappings:
		if subject.AddressGroupPortMappings == nil || len(subject.AddressGroupPortMappings.AddressGroupPortMappings) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем маппинги портов групп адресов
		mappings := make([]models.AddressGroupPortMapping, 0, len(subject.AddressGroupPortMappings.AddressGroupPortMappings))
		for _, m := range subject.AddressGroupPortMappings.AddressGroupPortMappings {
			mappings = append(mappings, convertAddressGroupPortMapping(m))
		}

		// Синхронизируем маппинги портов групп адресов с указанной операцией
		err = s.service.Sync(ctx, syncOp, mappings)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync address group port mappings")
		}

	case *netguardpb.SyncReq_RuleS2S:
		if subject.RuleS2S == nil || len(subject.RuleS2S.RuleS2S) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем правила s2s
		rules := make([]models.RuleS2S, 0, len(subject.RuleS2S.RuleS2S))
		for _, r := range subject.RuleS2S.RuleS2S {
			rules = append(rules, convertRuleS2S(r))
		}

		// Синхронизируем правила s2s с указанной операцией
		err = s.service.Sync(ctx, syncOp, rules)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync rule s2s")
		}

	case *netguardpb.SyncReq_ServiceAliases:
		if subject.ServiceAliases == nil || len(subject.ServiceAliases.ServiceAliases) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем алиасы сервисов
		aliases := make([]models.ServiceAlias, 0, len(subject.ServiceAliases.ServiceAliases))
		for _, a := range subject.ServiceAliases.ServiceAliases {
			aliases = append(aliases, convertServiceAlias(a))
		}

		// Синхронизируем алиасы сервисов с указанной операцией
		err = s.service.Sync(ctx, syncOp, aliases)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync service aliases")
		}

	case *netguardpb.SyncReq_IeagagRules:
		if subject.IeagagRules == nil || len(subject.IeagagRules.IeagagRules) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Convert IEAgAgRules
		rules := make([]models.IEAgAgRule, 0, len(subject.IeagagRules.IeagagRules))
		for _, r := range subject.IeagagRules.IeagagRules {
			rules = append(rules, convertIEAgAgRule(r))
		}

		// Sync IEAgAgRules with the specified operation
		err = s.service.Sync(ctx, syncOp, rules)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync IEAgAgRules")
		}

	case *netguardpb.SyncReq_AddressGroupBindingPolicies:
		if subject.AddressGroupBindingPolicies == nil || len(subject.AddressGroupBindingPolicies.AddressGroupBindingPolicies) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем политики привязки групп адресов
		policies := make([]models.AddressGroupBindingPolicy, 0, len(subject.AddressGroupBindingPolicies.AddressGroupBindingPolicies))
		for _, p := range subject.AddressGroupBindingPolicies.AddressGroupBindingPolicies {
			policies = append(policies, convertAddressGroupBindingPolicy(p))
		}

		// Синхронизируем политики привязки групп адресов с указанной операцией
		err = s.service.Sync(ctx, syncOp, policies)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync address group binding policies")
		}

	default:
		return nil, errors.New("subject not specified")
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
		Meta:        models.Meta{},
	}

	// copy meta if provided
	if svc.Meta != nil {
		result.Meta = models.Meta{
			UID:             svc.Meta.Uid,
			ResourceVersion: svc.Meta.ResourceVersion,
			Generation:      svc.Meta.Generation,
			Labels:          svc.Meta.Labels,
			Annotations:     svc.Meta.Annotations,
		}
		if svc.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(svc.Meta.CreationTs.AsTime())
		}
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
		SelfRef:       models.NewSelfRef(getSelfRef(ag.GetSelfRef())),
		DefaultAction: models.RuleAction(ag.DefaultAction.String()),
		Logs:          ag.Logs,
		Trace:         ag.Trace,
		Meta:          models.Meta{},
	}

	if ag.Meta != nil {
		result.Meta = models.Meta{
			UID:             ag.Meta.Uid,
			ResourceVersion: ag.Meta.ResourceVersion,
			Generation:      ag.Meta.Generation,
			Labels:          ag.Meta.Labels,
			Annotations:     ag.Meta.Annotations,
		}
		if ag.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(ag.Meta.CreationTs.AsTime())
		}
	}

	return result
}

func convertAddressGroupBinding(b *netguardpb.AddressGroupBinding) models.AddressGroupBinding {
	result := models.AddressGroupBinding{
		SelfRef: models.NewSelfRef(getSelfRef(b.GetSelfRef())),
		Meta:    models.Meta{},
	}

	// Convert ServiceRef
	result.ServiceRef = models.NewServiceRef(b.GetServiceRef().GetIdentifier().GetName(),
		models.WithNamespace(b.GetServiceRef().GetIdentifier().GetNamespace()))

	// Convert AddressGroupRef
	result.AddressGroupRef = models.NewAddressGroupRef(b.GetAddressGroupRef().GetIdentifier().GetName(),
		models.WithNamespace(b.GetAddressGroupRef().GetIdentifier().GetNamespace()))

	// Copy Meta if presented
	if b.Meta != nil {
		result.Meta = models.Meta{
			UID:             b.Meta.Uid,
			ResourceVersion: b.Meta.ResourceVersion,
			Generation:      b.Meta.Generation,
			Labels:          b.Meta.Labels,
			Annotations:     b.Meta.Annotations,
		}
		if b.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(b.Meta.CreationTs.AsTime())
		}
	}

	return result
}

func convertAddressGroupPortMapping(m *netguardpb.AddressGroupPortMapping) models.AddressGroupPortMapping {
	result := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(getSelfRef(m.GetSelfRef())),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{},
		Meta:        models.Meta{},
	}

	// Copy Meta
	if m.Meta != nil {
		result.Meta = models.Meta{
			UID:             m.Meta.Uid,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		}
		if m.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(m.Meta.CreationTs.AsTime())
		}
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
		Meta:    models.Meta{},
	}

	// Правильная конвертация Traffic
	switch r.Traffic {
	case commonpb.Traffic_Ingress:
		result.Traffic = models.INGRESS
	case commonpb.Traffic_Egress:
		result.Traffic = models.EGRESS
	}

	// Convert ServiceLocalRef
	result.ServiceLocalRef = models.ServiceAliasRef{
		ResourceIdentifier: models.NewResourceIdentifier(r.ServiceLocalRef.Identifier.Name, models.WithNamespace(r.GetServiceLocalRef().GetIdentifier().GetNamespace())),
	}

	// Convert ServiceRef
	result.ServiceRef = models.ServiceAliasRef{
		ResourceIdentifier: models.NewResourceIdentifier(r.ServiceRef.Identifier.Name, models.WithNamespace(r.GetServiceRef().GetIdentifier().GetNamespace())),
	}

	if r.Meta != nil {
		result.Meta = models.Meta{
			UID:             r.Meta.Uid,
			ResourceVersion: r.Meta.ResourceVersion,
			Generation:      r.Meta.Generation,
			Labels:          r.Meta.Labels,
			Annotations:     r.Meta.Annotations,
		}
		if r.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(r.Meta.CreationTs.AsTime())
		}
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
		Meta: &netguardpb.Meta{
			Uid:                svc.Meta.UID,
			ResourceVersion:    svc.Meta.ResourceVersion,
			Generation:         svc.Meta.Generation,
			Labels:             svc.Meta.Labels,
			Annotations:        svc.Meta.Annotations,
			Conditions:         models.K8sConditionsToProto(svc.Meta.Conditions), // ✅ ИСПРАВЛЕНО: добавляем conditions
			ObservedGeneration: svc.Meta.ObservedGeneration,
		},
	}

	if !svc.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(svc.Meta.CreationTS.Time)
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
	// Конвертация RuleAction string в protobuf enum
	var defaultAction netguardpb.RuleAction
	switch ag.DefaultAction {
	case models.ActionAccept:
		defaultAction = netguardpb.RuleAction_ACCEPT
	case models.ActionDrop:
		defaultAction = netguardpb.RuleAction_DROP
	default:
		defaultAction = netguardpb.RuleAction_ACCEPT // default
	}

	result := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      ag.ResourceIdentifier.Name,
			Namespace: ag.ResourceIdentifier.Namespace,
		},
		DefaultAction: defaultAction,
		Logs:          ag.Logs,
		Trace:         ag.Trace,
		Meta: &netguardpb.Meta{
			Uid:                ag.Meta.UID,
			ResourceVersion:    ag.Meta.ResourceVersion,
			Generation:         ag.Meta.Generation,
			Labels:             ag.Meta.Labels,
			Annotations:        ag.Meta.Annotations,
			Conditions:         models.K8sConditionsToProto(ag.Meta.Conditions), // ✅ ИСПРАВЛЕНО
			ObservedGeneration: ag.Meta.ObservedGeneration,
		},
	}

	if !ag.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(ag.Meta.CreationTS.Time)
	}

	return result
}

func convertAddressGroupBindingToPB(b models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	pb := &netguardpb.AddressGroupBinding{
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

	// Meta
	pb.Meta = &netguardpb.Meta{
		Uid:                b.Meta.UID,
		ResourceVersion:    b.Meta.ResourceVersion,
		Generation:         b.Meta.Generation,
		Labels:             b.Meta.Labels,
		Annotations:        b.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(b.Meta.Conditions), // ✅ ИСПРАВЛЕНО
		ObservedGeneration: b.Meta.ObservedGeneration,
	}
	if !b.Meta.CreationTS.IsZero() {
		pb.Meta.CreationTs = timestamppb.New(b.Meta.CreationTS.Time)
	}

	return pb
}

func convertAddressGroupPortMappingToPB(m models.AddressGroupPortMapping) *netguardpb.AddressGroupPortMapping {
	result := &netguardpb.AddressGroupPortMapping{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
	}

	// Meta
	result.Meta = &netguardpb.Meta{
		Uid:                m.Meta.UID,
		ResourceVersion:    m.Meta.ResourceVersion,
		Generation:         m.Meta.Generation,
		Labels:             m.Meta.Labels,
		Annotations:        m.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(m.Meta.Conditions), // ✅ ИСПРАВЛЕНО
		ObservedGeneration: m.Meta.ObservedGeneration,
	}
	if !m.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
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
	pb := &netguardpb.RuleS2S{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      r.ResourceIdentifier.Name,
			Namespace: r.ResourceIdentifier.Namespace,
		},
		// Traffic set later
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
	// traffic enum conversion
	if r.Traffic == models.EGRESS {
		pb.Traffic = commonpb.Traffic_Egress
	} else {
		pb.Traffic = commonpb.Traffic_Ingress
	}

	pb.Meta = &netguardpb.Meta{
		Uid:                r.Meta.UID,
		ResourceVersion:    r.Meta.ResourceVersion,
		Generation:         r.Meta.Generation,
		Labels:             r.Meta.Labels,
		Annotations:        r.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(r.Meta.Conditions), // ✅ ИСПРАВЛЕНО
		ObservedGeneration: r.Meta.ObservedGeneration,
	}
	if !r.Meta.CreationTS.IsZero() {
		pb.Meta.CreationTs = timestamppb.New(r.Meta.CreationTS.Time)
	}

	return pb
}

func convertServiceAliasToPB(a models.ServiceAlias) *netguardpb.ServiceAlias {
	pb := &netguardpb.ServiceAlias{
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
		Meta: &netguardpb.Meta{
			Uid:                a.Meta.UID,
			ResourceVersion:    a.Meta.ResourceVersion,
			Generation:         a.Meta.Generation,
			Labels:             a.Meta.Labels,
			Annotations:        a.Meta.Annotations,
			Conditions:         models.K8sConditionsToProto(a.Meta.Conditions), // ✅ ИСПРАВЛЕНО
			ObservedGeneration: a.Meta.ObservedGeneration,
		},
	}
	if !a.Meta.CreationTS.IsZero() {
		pb.Meta.CreationTs = timestamppb.New(a.Meta.CreationTS.Time)
	}
	return pb
}

func convertServiceAlias(a *netguardpb.ServiceAlias) models.ServiceAlias {
	alias := models.ServiceAlias{
		SelfRef: models.NewSelfRef(getSelfRef(a.GetSelfRef())),
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(a.GetServiceRef().GetIdentifier().GetName(), models.WithNamespace(a.GetServiceRef().GetIdentifier().GetNamespace())),
		},
		Meta: models.Meta{},
	}
	if a.Meta != nil {
		alias.Meta = models.Meta{
			UID:             a.Meta.Uid,
			ResourceVersion: a.Meta.ResourceVersion,
			Generation:      a.Meta.Generation,
			Labels:          a.Meta.Labels,
			Annotations:     a.Meta.Annotations,
		}
		if a.Meta.CreationTs != nil {
			alias.Meta.CreationTS = metav1.NewTime(a.Meta.CreationTs.AsTime())
		}
	}
	return alias
}

// ListIEAgAgRules gets list of IEAgAgRules
func (s *NetguardServiceServer) ListIEAgAgRules(ctx context.Context, req *netguardpb.ListIEAgAgRulesReq) (*netguardpb.ListIEAgAgRulesResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	rules, err := s.service.GetIEAgAgRules(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IEAgAgRules")
	}

	items := make([]*netguardpb.IEAgAgRule, 0, len(rules))
	for _, rule := range rules {
		items = append(items, convertIEAgAgRuleToPB(rule))
	}

	return &netguardpb.ListIEAgAgRulesResp{
		Items: items,
	}, nil
}

// GetIEAgAgRule gets a specific IEAgAgRule by ID
func (s *NetguardServiceServer) GetIEAgAgRule(ctx context.Context, req *netguardpb.GetIEAgAgRuleReq) (*netguardpb.GetIEAgAgRuleResp, error) {
	id := idFromReq(req.GetIdentifier())
	rule, err := s.service.GetIEAgAgRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IEAgAgRule")
	}

	return &netguardpb.GetIEAgAgRuleResp{
		IeagagRule: convertIEAgAgRuleToPB(*rule),
	}, nil
}

func convertIEAgAgRuleToPB(rule models.IEAgAgRule) *netguardpb.IEAgAgRule {
	var transport commonpb.Networks_NetIP_Transport
	switch rule.Transport {
	case models.TCP:
		transport = commonpb.Networks_NetIP_TCP
	case models.UDP:
		transport = commonpb.Networks_NetIP_UDP
	}

	var traffic commonpb.Traffic
	switch rule.Traffic {
	case models.INGRESS:
		traffic = commonpb.Traffic_Ingress
	case models.EGRESS:
		traffic = commonpb.Traffic_Egress
	}

	result := &netguardpb.IEAgAgRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Transport: transport,
		Traffic:   traffic,
		AddressGroupLocal: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      rule.AddressGroupLocal.ResourceIdentifier.Name,
				Namespace: rule.AddressGroupLocal.ResourceIdentifier.Namespace,
			},
		},
		AddressGroup: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      rule.AddressGroup.ResourceIdentifier.Name,
				Namespace: rule.AddressGroup.ResourceIdentifier.Namespace,
			},
		},
		Action:   netguardpb.RuleAction(netguardpb.RuleAction_value[string(rule.Action)]),
		Logs:     rule.Logs,
		Priority: rule.Priority,
	}

	// Populate Meta
	result.Meta = &netguardpb.Meta{
		Uid:                rule.Meta.UID,
		ResourceVersion:    rule.Meta.ResourceVersion,
		Generation:         rule.Meta.Generation,
		Labels:             rule.Meta.Labels,
		Annotations:        rule.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(rule.Meta.Conditions), // ✅ ИСПРАВЛЕНО
		ObservedGeneration: rule.Meta.ObservedGeneration,
	}
	if !rule.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(rule.Meta.CreationTS.Time)
	}

	// Convert ports
	for _, p := range rule.Ports {
		result.Ports = append(result.Ports, &netguardpb.PortSpec{
			Source:      p.Source,
			Destination: p.Destination,
		})
	}

	return result
}

func convertIEAgAgRule(rule *netguardpb.IEAgAgRule) models.IEAgAgRule {
	result := models.IEAgAgRule{
		SelfRef:   models.NewSelfRef(getSelfRef(rule.GetSelfRef())),
		Transport: models.TransportProtocol(rule.Transport.String()),
		Traffic:   models.Traffic(rule.Traffic.String()),
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(rule.GetAddressGroupLocal().GetIdentifier().GetName(),
				models.WithNamespace(rule.GetAddressGroupLocal().GetIdentifier().GetNamespace())),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(rule.GetAddressGroup().GetIdentifier().GetName(),
				models.WithNamespace(rule.GetAddressGroup().GetIdentifier().GetNamespace())),
		},
		Action:   models.RuleAction(rule.Action),
		Logs:     rule.Logs,
		Priority: rule.Priority,
		Meta:     models.Meta{},
	}

	// Copy Meta from proto
	if rule.Meta != nil {
		result.Meta = models.Meta{
			UID:             rule.Meta.Uid,
			ResourceVersion: rule.Meta.ResourceVersion,
			Generation:      rule.Meta.Generation,
			Labels:          rule.Meta.Labels,
			Annotations:     rule.Meta.Annotations,
		}
		if rule.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(rule.Meta.CreationTs.AsTime())
		}
	}

	// Convert ports
	for _, p := range rule.Ports {
		result.Ports = append(result.Ports, models.PortSpec{
			Source:      p.Source,
			Destination: p.Destination,
		})
	}

	return result
}

func convertAddressGroupBindingPolicy(policy *netguardpb.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	result := models.AddressGroupBindingPolicy{
		SelfRef: models.NewSelfRef(getSelfRef(policy.GetSelfRef())),
		Meta:    models.Meta{},
	}

	// Convert ServiceRef
	result.ServiceRef = models.NewServiceRef(policy.GetServiceRef().GetIdentifier().GetName(),
		models.WithNamespace(policy.GetServiceRef().GetIdentifier().GetNamespace()))

	// Convert AddressGroupRef
	result.AddressGroupRef = models.NewAddressGroupRef(policy.GetAddressGroupRef().GetIdentifier().GetName(),
		models.WithNamespace(policy.GetAddressGroupRef().GetIdentifier().GetNamespace()))

	// Copy Meta information if present
	if policy.Meta != nil {
		result.Meta = models.Meta{
			UID:             policy.Meta.Uid,
			ResourceVersion: policy.Meta.ResourceVersion,
			Generation:      policy.Meta.Generation,
			Labels:          policy.Meta.Labels,
			Annotations:     policy.Meta.Annotations,
		}
		if policy.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(policy.Meta.CreationTs.AsTime())
		}
	}

	return result
}

func convertAddressGroupBindingPolicyToPB(policy models.AddressGroupBindingPolicy) *netguardpb.AddressGroupBindingPolicy {
	pbPolicy := &netguardpb.AddressGroupBindingPolicy{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      policy.Name,
			Namespace: policy.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      policy.ServiceRef.Name,
				Namespace: policy.ServiceRef.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      policy.AddressGroupRef.Name,
				Namespace: policy.AddressGroupRef.Namespace,
			},
		},
	}

	// Populate Meta information
	pbPolicy.Meta = &netguardpb.Meta{
		Uid:                policy.Meta.UID,
		ResourceVersion:    policy.Meta.ResourceVersion,
		Generation:         policy.Meta.Generation,
		Labels:             policy.Meta.Labels,
		Annotations:        policy.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(policy.Meta.Conditions), // ✅ ИСПРАВЛЕНО
		ObservedGeneration: policy.Meta.ObservedGeneration,
	}
	if !policy.Meta.CreationTS.IsZero() {
		pbPolicy.Meta.CreationTs = timestamppb.New(policy.Meta.CreationTS.Time)
	}

	return pbPolicy
}

// ListAddressGroupBindingPolicies gets list of address group binding policies
func (s *NetguardServiceServer) ListAddressGroupBindingPolicies(ctx context.Context, req *netguardpb.ListAddressGroupBindingPoliciesReq) (*netguardpb.ListAddressGroupBindingPoliciesResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	policies, err := s.service.GetAddressGroupBindingPolicies(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding policies")
	}

	items := make([]*netguardpb.AddressGroupBindingPolicy, 0, len(policies))
	for _, policy := range policies {
		items = append(items, convertAddressGroupBindingPolicyToPB(policy))
	}

	return &netguardpb.ListAddressGroupBindingPoliciesResp{
		Items: items,
	}, nil
}

// GetAddressGroupBindingPolicy gets a specific address group binding policy by ID
func (s *NetguardServiceServer) GetAddressGroupBindingPolicy(ctx context.Context, req *netguardpb.GetAddressGroupBindingPolicyReq) (*netguardpb.GetAddressGroupBindingPolicyResp, error) {
	id := idFromReq(req.GetIdentifier())
	policy, err := s.service.GetAddressGroupBindingPolicyByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address group binding policy")
	}

	return &netguardpb.GetAddressGroupBindingPolicyResp{
		AddressGroupBindingPolicy: convertAddressGroupBindingPolicyToPB(*policy),
	}, nil
}
