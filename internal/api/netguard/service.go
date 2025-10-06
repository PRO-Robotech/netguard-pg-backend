package netguard

import (
	"context"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// commonpb "github.com/H-BF/protos/pkg/api/common" - replaced with local types
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// NetguardServiceServer implements the NetguardService gRPC service
type NetguardServiceServer struct {
	netguardpb.UnimplementedNetguardServiceServer
	service *services.NetguardFacade
}

// NewNetguardServiceServer creates a new NetguardServiceServer
func NewNetguardServiceServer(service *services.NetguardFacade) *NetguardServiceServer {
	return &NetguardServiceServer{
		service: service,
	}
}

// convertSyncOp преобразует proto SyncOp в models.SyncOp
func convertSyncOp(protoSyncOp netguardpb.SyncOp) models.SyncOp {
	return models.ProtoToSyncOp(int32(protoSyncOp))
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
			convertedService := convertService(svc)

			services = append(services, convertedService)
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

		bindings := make([]models.AddressGroupBinding, 0, len(subject.AddressGroupBindings.AddressGroupBindings))
		for _, b := range subject.AddressGroupBindings.AddressGroupBindings {
			binding := convertAddressGroupBinding(b)
			bindings = append(bindings, binding)
		}

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
			rule := client.ConvertIEAgAgRuleFromProto(r)
			rules = append(rules, rule)
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

	case *netguardpb.SyncReq_Networks:
		if subject.Networks == nil || len(subject.Networks.Networks) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем сети
		networks := make([]models.Network, 0, len(subject.Networks.Networks))
		for _, n := range subject.Networks.Networks {
			networks = append(networks, convertNetwork(n))
		}

		// Синхронизируем сети с указанной операцией
		err = s.service.Sync(ctx, syncOp, networks)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync networks")
		}

	case *netguardpb.SyncReq_NetworkBindings:
		if subject.NetworkBindings == nil || len(subject.NetworkBindings.NetworkBindings) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем привязки сетей
		bindings := make([]models.NetworkBinding, 0, len(subject.NetworkBindings.NetworkBindings))
		for _, b := range subject.NetworkBindings.NetworkBindings {
			bindings = append(bindings, convertNetworkBinding(b))
		}

		// Синхронизируем привязки сетей с указанной операцией
		err = s.service.Sync(ctx, syncOp, bindings)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync network bindings")
		}

	case *netguardpb.SyncReq_Hosts:
		if subject.Hosts == nil || len(subject.Hosts.Hosts) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем хосты
		hosts := make([]models.Host, 0, len(subject.Hosts.Hosts))
		for _, h := range subject.Hosts.Hosts {
			hosts = append(hosts, convertHost(h))
		}

		// Синхронизируем хосты с указанной операцией
		err = s.service.Sync(ctx, syncOp, hosts)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync hosts")
		}

	case *netguardpb.SyncReq_HostBindings:
		if subject.HostBindings == nil || len(subject.HostBindings.HostBindings) == 0 {
			return &emptypb.Empty{}, nil
		}

		// Конвертируем привязки хостов
		bindings := make([]models.HostBinding, 0, len(subject.HostBindings.HostBindings))
		for _, b := range subject.HostBindings.HostBindings {
			bindings = append(bindings, convertHostBinding(b))
		}

		err = s.service.Sync(ctx, syncOp, bindings)
		if err != nil {
			return nil, errors.Wrap(err, "failed to sync host bindings")
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

	pbAddressGroup := convertAddressGroupToPB(*addressGroup)

	return &netguardpb.GetAddressGroupResp{
		AddressGroup: pbAddressGroup,
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

	// Convert address groups with nil-safe access
	for _, ag := range svc.AddressGroups {
		var agName, agNamespace string
		if agId := ag.GetIdentifier(); agId != nil {
			agName = agId.GetName()
			agNamespace = agId.GetNamespace()
		}
		// Skip empty AddressGroup references
		if agName != "" {
			ref := models.NewAddressGroupRef(agName, models.WithNamespace(agNamespace))
			result.AddressGroups = append(result.AddressGroups, ref)
		}
	}

	// Convert AggregatedAddressGroups from proto to domain
	if len(svc.AggregatedAddressGroups) > 0 {
		result.AggregatedAddressGroups = make([]models.AddressGroupReference, len(svc.AggregatedAddressGroups))
		for i, agRef := range svc.AggregatedAddressGroups {
			result.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref: v1beta1.NamespacedObjectReference{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: agRef.Ref.ApiVersion,
						Kind:       agRef.Ref.Kind,
						Name:       agRef.Ref.Name,
					},
					Namespace: agRef.Ref.Namespace,
				},
				Source: convertAGRegistrationSourceFromPB(agRef.Source),
			}
		}
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

	// Convert hosts field (NEW: hosts belonging to this address group)
	if len(ag.Hosts) > 0 {
		result.Hosts = make([]v1beta1.ObjectReference, len(ag.Hosts))
		for i, host := range ag.Hosts {
			result.Hosts[i] = v1beta1.ObjectReference{
				APIVersion: host.ApiVersion,
				Kind:       host.Kind,
				Name:       host.Name,
			}
		}
	}

	if len(ag.AggregatedHosts) > 0 {
		result.AggregatedHosts = make([]models.HostReference, len(ag.AggregatedHosts))
		for i, hostRef := range ag.AggregatedHosts {
			result.AggregatedHosts[i] = models.HostReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: hostRef.Ref.ApiVersion,
					Kind:       hostRef.Ref.Kind,
					Name:       hostRef.Ref.Name,
				},
				UUID:   hostRef.Uuid,
				Source: convertHostRegistrationSourceFromPB(hostRef.Source),
			}
		}
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

	// Convert ServiceRef with nil-safe access
	var serviceName, serviceNamespace string
	if svcRef := b.GetServiceRef(); svcRef != nil {
		if svcId := svcRef.GetIdentifier(); svcId != nil {
			serviceName = svcId.GetName()
			serviceNamespace = svcId.GetNamespace()
		}
	}
	if serviceName == "" {
		return result
	}

	result.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))

	// Convert AddressGroupRef with nil-safe access
	var agName, agNamespace string
	if agRef := b.GetAddressGroupRef(); agRef != nil {
		if agId := agRef.GetIdentifier(); agId != nil {
			agName = agId.GetName()
			agNamespace = agId.GetNamespace()
		}
	}

	if agName == "" {
		return result
	}

	result.AddressGroupRef = models.NewAddressGroupRef(agName, models.WithNamespace(agNamespace))

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
		spr := models.NewServiceRef(
			ap.Identifier.Name,
			models.WithNamespace(ap.GetIdentifier().GetNamespace()),
		)
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

	switch r.Traffic {
	case netguardpb.Traffic_Ingress:
		result.Traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		result.Traffic = models.EGRESS
	}

	result.Trace = r.Trace

	var localName, localNamespace string
	if localRef := r.GetServiceLocalRef(); localRef != nil {
		if objRef := localRef.GetObjectRef(); objRef != nil {
			localName = objRef.GetName()
			localNamespace = objRef.GetNamespace()
		} else if localId := localRef.GetIdentifier(); localId != nil {
			localName = localId.GetName()
			localNamespace = localId.GetNamespace()
		}
	}
	if localName == "" {
		return result // Skip conversion if ServiceLocalRef is incomplete
	}
	result.ServiceLocalRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
			Name:       localName,
		},
		Namespace: localNamespace,
	}

	// Convert ServiceRef with nil-safe access
	var serviceName, serviceNamespace string
	if svcRef := r.GetServiceRef(); svcRef != nil {
		if objRef := svcRef.GetObjectRef(); objRef != nil {
			serviceName = objRef.GetName()
			serviceNamespace = objRef.GetNamespace()
		} else if svcId := svcRef.GetIdentifier(); svcId != nil {
			serviceName = svcId.GetName()
			serviceNamespace = svcId.GetNamespace()
		}
	}
	if serviceName == "" {
		return result // Skip conversion if ServiceRef is incomplete
	}
	result.ServiceRef = v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
			Name:       serviceName,
		},
		Namespace: serviceNamespace,
	}

	if len(r.IeagAgRuleObjectRefs) > 0 {
		result.IEAgAgRuleRefs = make([]v1beta1.NamespacedObjectReference, len(r.IeagAgRuleObjectRefs))
		for i, ref := range r.IeagAgRuleObjectRefs {
			result.IEAgAgRuleRefs[i] = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: ref.ApiVersion,
					Kind:       ref.Kind,
					Name:       ref.Name,
				},
				Namespace: ref.Namespace,
			}
		}
	} else if len(r.IeagAgRuleRefs) > 0 {
		result.IEAgAgRuleRefs = make([]v1beta1.NamespacedObjectReference, len(r.IeagAgRuleRefs))
		for i, ref := range r.IeagAgRuleRefs {
			result.IEAgAgRuleRefs[i] = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "IEAgAgRule",
					Name:       ref.Name,
				},
				Namespace: ref.Namespace,
			}
		}
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
			Conditions:         models.K8sConditionsToProto(svc.Meta.Conditions),
			ObservedGeneration: svc.Meta.ObservedGeneration,
		},
	}

	if !svc.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(svc.Meta.CreationTS.Time)
	}

	// Convert ingress ports
	for _, p := range svc.IngressPorts {
		var proto netguardpb.Networks_NetIP_Transport
		switch p.Protocol {
		case models.TCP:
			proto = netguardpb.Networks_NetIP_TCP
		case models.UDP:
			proto = netguardpb.Networks_NetIP_UDP
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
				Name:      ag.Name,
				Namespace: ag.Namespace,
			},
		})
	}

	// Convert AggregatedAddressGroups from domain to proto
	if len(svc.AggregatedAddressGroups) > 0 {
		result.AggregatedAddressGroups = make([]*netguardpb.AddressGroupReference, len(svc.AggregatedAddressGroups))
		for i, agRef := range svc.AggregatedAddressGroups {
			result.AggregatedAddressGroups[i] = &netguardpb.AddressGroupReference{
				Ref: &netguardpb.NamespacedObjectReference{
					ApiVersion: agRef.Ref.APIVersion,
					Kind:       agRef.Ref.Kind,
					Name:       agRef.Ref.Name,
					Namespace:  agRef.Ref.Namespace,
				},
				Source: convertAGRegistrationSourceToPB(agRef.Source),
			}
		}
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
		DefaultAction:    defaultAction,
		Logs:             ag.Logs,
		Trace:            ag.Trace,
		AddressGroupName: ag.AddressGroupName,
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

	// Convert Networks list
	for _, networkItem := range ag.Networks {
		result.Networks = append(result.Networks, &netguardpb.NetworkItem{
			Name:       networkItem.Name,
			Cidr:       networkItem.CIDR,
			ApiVersion: networkItem.ApiVersion,
			Kind:       networkItem.Kind,
			Namespace:  networkItem.Namespace,
		})
	}

	// Convert hosts field (NEW: hosts belonging to this address group)
	if len(ag.Hosts) > 0 {
		result.Hosts = make([]*netguardpb.ObjectReference, len(ag.Hosts))
		for i, host := range ag.Hosts {
			result.Hosts[i] = &netguardpb.ObjectReference{
				ApiVersion: host.APIVersion,
				Kind:       host.Kind,
				Name:       host.Name,
			}
		}
	}

	// Convert AggregatedHosts field (NEW: aggregated hosts from database triggers)
	if len(ag.AggregatedHosts) > 0 {
		result.AggregatedHosts = make([]*netguardpb.HostReference, len(ag.AggregatedHosts))
		for i, hostRef := range ag.AggregatedHosts {
			result.AggregatedHosts[i] = &netguardpb.HostReference{
				Ref: &netguardpb.ObjectReference{
					ApiVersion: hostRef.ObjectReference.APIVersion,
					Kind:       hostRef.ObjectReference.Kind,
					Name:       hostRef.ObjectReference.Name,
				},
				Uuid:   hostRef.UUID,
				Source: convertHostRegistrationSourceToPB(hostRef.Source),
			}
		}
	}

	return result
}

// convertHostRegistrationSourceToPB converts domain HostRegistrationSource to protobuf enum
func convertHostRegistrationSourceToPB(source models.HostRegistrationSource) netguardpb.HostRegistrationSource {
	switch source {
	case models.HostSourceSpec:
		return netguardpb.HostRegistrationSource_HOST_SOURCE_SPEC
	case models.HostSourceBinding:
		return netguardpb.HostRegistrationSource_HOST_SOURCE_BINDING
	default:
		return netguardpb.HostRegistrationSource_HOST_SOURCE_SPEC // default
	}
}

// convertHostRegistrationSourceFromPB converts protobuf HostRegistrationSource to domain enum
func convertHostRegistrationSourceFromPB(source netguardpb.HostRegistrationSource) models.HostRegistrationSource {
	switch source {
	case netguardpb.HostRegistrationSource_HOST_SOURCE_SPEC:
		return models.HostSourceSpec
	case netguardpb.HostRegistrationSource_HOST_SOURCE_BINDING:
		return models.HostSourceBinding
	default:
		return models.HostSourceSpec // default
	}
}

// convertAGRegistrationSourceFromPB converts proto AddressGroupRegistrationSource to domain
func convertAGRegistrationSourceFromPB(source netguardpb.AddressGroupRegistrationSource) models.AddressGroupRegistrationSource {
	switch source {
	case netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC:
		return models.AddressGroupSourceSpec
	case netguardpb.AddressGroupRegistrationSource_AG_SOURCE_BINDING:
		return models.AddressGroupSourceBinding
	default:
		return models.AddressGroupSourceSpec // default
	}
}

// convertAGRegistrationSourceToPB converts domain AddressGroupRegistrationSource to proto
func convertAGRegistrationSourceToPB(source models.AddressGroupRegistrationSource) netguardpb.AddressGroupRegistrationSource {
	switch source {
	case models.AddressGroupSourceSpec:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC
	case models.AddressGroupSourceBinding:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_BINDING
	default:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC // default
	}
}

func convertAddressGroupBindingToPB(b models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	pb := &netguardpb.AddressGroupBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      b.ResourceIdentifier.Name,
			Namespace: b.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      b.ServiceRef.Name,
				Namespace: b.ServiceRef.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      b.AddressGroupRef.Name,
				Namespace: b.AddressGroupRef.Namespace,
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
		Conditions:         models.K8sConditionsToProto(b.Meta.Conditions),
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
		Conditions:         models.K8sConditionsToProto(m.Meta.Conditions),
		ObservedGeneration: m.Meta.ObservedGeneration,
	}
	if !m.Meta.CreationTS.IsZero() {
		result.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	// Convert access ports
	for srv, ap := range m.AccessPorts {
		spr := &netguardpb.ServicePortsRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      srv.Name,
				Namespace: srv.Namespace,
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
		ServiceLocalRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      r.ServiceLocalRef.Name,
				Namespace: r.ServiceLocalRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: r.ServiceLocalRef.APIVersion,
				Kind:       r.ServiceLocalRef.Kind,
				Name:       r.ServiceLocalRef.Name,
				Namespace:  r.ServiceLocalRef.Namespace,
			},
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      r.ServiceRef.Name,
				Namespace: r.ServiceRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: r.ServiceRef.APIVersion,
				Kind:       r.ServiceRef.Kind,
				Name:       r.ServiceRef.Name,
				Namespace:  r.ServiceRef.Namespace,
			},
		},
	}

	if len(r.IEAgAgRuleRefs) > 0 {
		pb.IeagAgRuleObjectRefs = make([]*netguardpb.NamespacedObjectReference, len(r.IEAgAgRuleRefs))
		for i, ref := range r.IEAgAgRuleRefs {
			pb.IeagAgRuleObjectRefs[i] = &netguardpb.NamespacedObjectReference{
				ApiVersion: ref.APIVersion,
				Kind:       ref.Kind,
				Name:       ref.Name,
				Namespace:  ref.Namespace,
			}
		}
		// Also provide legacy format for backward compatibility
		pb.IeagAgRuleRefs = make([]*netguardpb.ResourceIdentifier, len(r.IEAgAgRuleRefs))
		for i, ref := range r.IEAgAgRuleRefs {
			pb.IeagAgRuleRefs[i] = &netguardpb.ResourceIdentifier{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			}
		}
	}

	pb.Trace = r.Trace

	if r.Traffic == models.EGRESS {
		pb.Traffic = netguardpb.Traffic_Egress
	} else {
		pb.Traffic = netguardpb.Traffic_Ingress
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
				Name:      a.ServiceRef.Name,
				Namespace: a.ServiceRef.Namespace,
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
	// Convert ServiceRef with nil-safe access
	var serviceName, serviceNamespace string
	if svcRef := a.GetServiceRef(); svcRef != nil {
		if svcId := svcRef.GetIdentifier(); svcId != nil {
			serviceName = svcId.GetName()
			serviceNamespace = svcId.GetNamespace()
		}
	}
	if serviceName == "" {
		// Return partial object if ServiceRef is incomplete - let caller handle validation
		return models.ServiceAlias{
			SelfRef: models.NewSelfRef(getSelfRef(a.GetSelfRef())),
			Meta:    models.Meta{},
		}
	}

	alias := models.ServiceAlias{
		SelfRef:    models.NewSelfRef(getSelfRef(a.GetSelfRef())),
		ServiceRef: models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace)),
		Meta:       models.Meta{},
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

func convertActionToPB(action models.RuleAction) netguardpb.RuleAction {
	switch action {
	case models.ActionAccept:
		return netguardpb.RuleAction_ACCEPT
	case models.ActionDrop:
		return netguardpb.RuleAction_DROP
	default:
		return netguardpb.RuleAction_ACCEPT
	}
}

func convertIEAgAgRuleToPB(rule models.IEAgAgRule) *netguardpb.IEAgAgRule {

	var transport netguardpb.Networks_NetIP_Transport
	switch rule.Transport {
	case models.TCP:
		transport = netguardpb.Networks_NetIP_TCP
	case models.UDP:
		transport = netguardpb.Networks_NetIP_UDP
	default:
		transport = netguardpb.Networks_NetIP_TCP
	}

	var traffic netguardpb.Traffic
	switch rule.Traffic {
	case models.INGRESS:
		traffic = netguardpb.Traffic_Ingress
	case models.EGRESS:
		traffic = netguardpb.Traffic_Egress
	default:
		traffic = netguardpb.Traffic_Ingress
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
				Name:      rule.AddressGroupLocal.Name,
				Namespace: rule.AddressGroupLocal.Namespace,
			},
		},
		AddressGroup: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      rule.AddressGroup.Name,
				Namespace: rule.AddressGroup.Namespace,
			},
		},
		Action:   convertActionToPB(rule.Action),
		Logs:     rule.Logs,
		Priority: rule.Priority,
		Trace:    rule.Trace,
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

func convertAddressGroupBindingPolicy(policy *netguardpb.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	result := models.AddressGroupBindingPolicy{
		SelfRef: models.NewSelfRef(getSelfRef(policy.GetSelfRef())),
		Meta:    models.Meta{},
	}

	// Convert ServiceRef with nil-safe access
	var serviceName, serviceNamespace string
	if svcRef := policy.GetServiceRef(); svcRef != nil {
		if svcId := svcRef.GetIdentifier(); svcId != nil {
			serviceName = svcId.GetName()
			serviceNamespace = svcId.GetNamespace()
		}
	}
	if serviceName == "" {
		return result
	}

	result.ServiceRef = models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace))

	// Convert AddressGroupRef with nil-safe access
	var agName, agNamespace string
	if agRef := policy.GetAddressGroupRef(); agRef != nil {
		if agId := agRef.GetIdentifier(); agId != nil {
			agName = agId.GetName()
			agNamespace = agId.GetNamespace()
		}
	}

	if agName == "" {
		return result
	}

	result.AddressGroupRef = models.NewAddressGroupRef(agName, models.WithNamespace(agNamespace))

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

// ListNetworks gets list of networks
func (s *NetguardServiceServer) ListNetworks(ctx context.Context, req *netguardpb.ListNetworksReq) (*netguardpb.ListNetworksResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	networks, err := s.service.GetNetworks(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get networks")
	}

	items := make([]*netguardpb.Network, 0, len(networks))
	for _, network := range networks {
		items = append(items, convertNetworkToPB(network))
	}

	return &netguardpb.ListNetworksResp{
		Items: items,
	}, nil
}

// GetNetwork gets a specific network by ID
func (s *NetguardServiceServer) GetNetwork(ctx context.Context, req *netguardpb.GetNetworkReq) (*netguardpb.GetNetworkResp, error) {
	id := idFromReq(req.GetIdentifier())
	network, err := s.service.GetNetworkByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network")
	}

	if network == nil {
		return nil, errors.New("network not found")
	}

	pbNetwork := convertNetworkToPB(*network)

	return &netguardpb.GetNetworkResp{
		Network: pbNetwork,
	}, nil
}

// ListNetworkBindings gets list of network bindings
func (s *NetguardServiceServer) ListNetworkBindings(ctx context.Context, req *netguardpb.ListNetworkBindingsReq) (*netguardpb.ListNetworkBindingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	bindings, err := s.service.GetNetworkBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network bindings")
	}

	items := make([]*netguardpb.NetworkBinding, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, convertNetworkBindingToPB(binding))
	}

	return &netguardpb.ListNetworkBindingsResp{
		Items: items,
	}, nil
}

// GetNetworkBinding gets a specific network binding by ID
func (s *NetguardServiceServer) GetNetworkBinding(ctx context.Context, req *netguardpb.GetNetworkBindingReq) (*netguardpb.GetNetworkBindingResp, error) {
	id := idFromReq(req.GetIdentifier())
	binding, err := s.service.GetNetworkBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network binding")
	}

	if binding == nil {
		return nil, errors.New("network binding not found")
	}

	return &netguardpb.GetNetworkBindingResp{
		NetworkBinding: convertNetworkBindingToPB(*binding),
	}, nil
}

// convertNetwork converts proto Network to domain Network
func convertNetwork(network *netguardpb.Network) models.Network {
	result := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      network.GetSelfRef().GetName(),
				Namespace: network.GetSelfRef().GetNamespace(),
			},
		},
		CIDR: network.Cidr,
		Meta: models.Meta{},
	}

	// Copy meta if provided
	if network.Meta != nil {
		result.Meta = models.Meta{
			UID:             network.Meta.Uid,
			ResourceVersion: network.Meta.ResourceVersion,
			Generation:      network.Meta.Generation,
			Labels:          network.Meta.Labels,
			Annotations:     network.Meta.Annotations,
			Conditions:      models.ProtoConditionsToK8s(network.Meta.Conditions),
		}
		if network.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(network.Meta.CreationTs.AsTime())
		}
	}

	return result
}

// convertNetworkToPB converts domain Network to proto Network
func convertNetworkToPB(network models.Network) *netguardpb.Network {
	pbNetwork := &netguardpb.Network{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      network.Name,
			Namespace: network.Namespace,
		},
		Cidr: network.CIDR,
	}

	// Populate Meta information
	pbNetwork.Meta = &netguardpb.Meta{
		Uid:                network.Meta.UID,
		ResourceVersion:    network.Meta.ResourceVersion,
		Generation:         network.Meta.Generation,
		Labels:             network.Meta.Labels,
		Annotations:        network.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(network.Meta.Conditions),
		ObservedGeneration: network.Meta.ObservedGeneration,
	}
	if !network.Meta.CreationTS.IsZero() {
		pbNetwork.Meta.CreationTs = timestamppb.New(network.Meta.CreationTS.Time)
	}

	// Add status fields
	pbNetwork.IsBound = network.IsBound

	if network.BindingRef != nil {
		pbNetwork.BindingRef = &netguardpb.ObjectReference{
			ApiVersion: network.BindingRef.APIVersion,
			Kind:       network.BindingRef.Kind,
			Name:       network.BindingRef.Name,
		}
	}

	if network.AddressGroupRef != nil {
		pbNetwork.AddressGroupRef = &netguardpb.NamespacedObjectReference{
			ApiVersion: network.AddressGroupRef.APIVersion,
			Kind:       network.AddressGroupRef.Kind,
			Name:       network.AddressGroupRef.Name,
			Namespace:  network.Namespace, // Use Network's namespace
		}
	}

	return pbNetwork
}

// convertNetworkBinding converts proto NetworkBinding to domain NetworkBinding
func convertNetworkBinding(binding *netguardpb.NetworkBinding) models.NetworkBinding {
	result := models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      binding.GetSelfRef().GetName(),
				Namespace: binding.GetSelfRef().GetNamespace(),
			},
		},
		Meta: models.Meta{},
	}

	// Convert NetworkRef with nil-safe access
	var networkName string
	if netRef := binding.GetNetworkRef(); netRef != nil {
		networkName = netRef.GetName()
	}
	if networkName == "" {
		return result // Skip conversion if NetworkRef is incomplete
	}
	result.NetworkRef = v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "Network",
		Name:       networkName,
	}

	// Convert AddressGroupRef with nil-safe access
	var agName string
	if agRef := binding.GetAddressGroupRef(); agRef != nil {
		agName = agRef.GetName()
	}
	if agName == "" {
		return result // Skip conversion if AddressGroupRef is incomplete
	}
	result.AddressGroupRef = v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       agName,
	}

	// Convert NetworkItem if present
	if binding.NetworkItem != nil {
		result.NetworkItem = models.NetworkItem{
			Name: binding.NetworkItem.Name,
			CIDR: binding.NetworkItem.Cidr,
		}
	}

	// Copy Meta if presented
	if binding.Meta != nil {
		result.Meta = models.Meta{
			UID:             binding.Meta.Uid,
			ResourceVersion: binding.Meta.ResourceVersion,
			Generation:      binding.Meta.Generation,
			Labels:          binding.Meta.Labels,
			Annotations:     binding.Meta.Annotations,
			Conditions:      models.ProtoConditionsToK8s(binding.Meta.Conditions),
		}
		if binding.Meta.CreationTs != nil {
			result.Meta.CreationTS = metav1.NewTime(binding.Meta.CreationTs.AsTime())
		}
	}

	return result
}

// convertNetworkBindingToPB converts domain NetworkBinding to proto NetworkBinding
func convertNetworkBindingToPB(binding models.NetworkBinding) *netguardpb.NetworkBinding {
	pbBinding := &netguardpb.NetworkBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},
		NetworkRef: &netguardpb.ObjectReference{
			ApiVersion: binding.NetworkRef.APIVersion,
			Kind:       binding.NetworkRef.Kind,
			Name:       binding.NetworkRef.Name,
		},
		AddressGroupRef: &netguardpb.ObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
		},
	}

	// Convert NetworkItem
	pbBinding.NetworkItem = &netguardpb.NetworkItem{
		Name: binding.NetworkItem.Name,
		Cidr: binding.NetworkItem.CIDR,
	}

	// Populate Meta information
	pbBinding.Meta = &netguardpb.Meta{
		Uid:                binding.Meta.UID,
		ResourceVersion:    binding.Meta.ResourceVersion,
		Generation:         binding.Meta.Generation,
		Labels:             binding.Meta.Labels,
		Annotations:        binding.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(binding.Meta.Conditions),
		ObservedGeneration: binding.Meta.ObservedGeneration,
	}
	if !binding.Meta.CreationTS.IsZero() {
		pbBinding.Meta.CreationTs = timestamppb.New(binding.Meta.CreationTS.Time)
	}

	return pbBinding
}

// ListHosts gets list of hosts
func (s *NetguardServiceServer) ListHosts(ctx context.Context, req *netguardpb.ListHostsReq) (*netguardpb.ListHostsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	hosts, err := s.service.GetHosts(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get hosts")
	}

	pbHosts := make([]*netguardpb.Host, 0, len(hosts))
	for _, host := range hosts {
		pbHosts = append(pbHosts, convertHostToPB(host))
	}

	return &netguardpb.ListHostsResp{
		Items: pbHosts,
	}, nil
}

// GetHost gets a host by identifier
func (s *NetguardServiceServer) GetHost(ctx context.Context, req *netguardpb.GetHostReq) (*netguardpb.GetHostResp, error) {
	id := models.NewResourceIdentifier(req.Identifier.Name, models.WithNamespace(req.Identifier.Namespace))

	host, err := s.service.GetHostByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host")
	}

	return &netguardpb.GetHostResp{
		Host: convertHostToPB(*host),
	}, nil
}

// ListHostBindings gets list of host bindings
func (s *NetguardServiceServer) ListHostBindings(ctx context.Context, req *netguardpb.ListHostBindingsReq) (*netguardpb.ListHostBindingsResp, error) {
	var scope ports.Scope = ports.EmptyScope{}
	if len(req.Identifiers) > 0 {
		identifiers := make([]models.ResourceIdentifier, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			identifiers = append(identifiers, models.NewResourceIdentifier(id.Name, models.WithNamespace(id.Namespace)))
		}
		scope = ports.NewResourceIdentifierScope(identifiers...)
	}

	hostBindings, err := s.service.GetHostBindings(ctx, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host bindings")
	}

	pbBindings := make([]*netguardpb.HostBinding, 0, len(hostBindings))
	for _, binding := range hostBindings {
		pbBindings = append(pbBindings, convertHostBindingToPB(binding))
	}

	return &netguardpb.ListHostBindingsResp{
		Items: pbBindings,
	}, nil
}

// GetHostBinding gets a host binding by identifier
func (s *NetguardServiceServer) GetHostBinding(ctx context.Context, req *netguardpb.GetHostBindingReq) (*netguardpb.GetHostBindingResp, error) {
	id := models.NewResourceIdentifier(req.Identifier.Name, models.WithNamespace(req.Identifier.Namespace))

	hostBinding, err := s.service.GetHostBindingByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get host binding")
	}

	return &netguardpb.GetHostBindingResp{
		HostBinding: convertHostBindingToPB(*hostBinding),
	}, nil
}

// convertHost converts proto Host to domain Host
func convertHost(protoHost *netguardpb.Host) models.Host {
	host := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoHost.SelfRef.Name,
				Namespace: protoHost.SelfRef.Namespace,
			},
		},
		UUID: protoHost.Uuid,

		// Status fields
		HostName:         protoHost.HostNameSync,
		AddressGroupName: protoHost.AddressGroupName,
		IsBound:          protoHost.IsBound,
	}

	// Set binding reference if present
	if protoHost.BindingRef != nil {
		host.BindingRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.BindingRef.ApiVersion,
			Kind:       protoHost.BindingRef.Kind,
			Name:       protoHost.BindingRef.Name,
		}
	}

	// Set address group reference if present
	if protoHost.AddressGroupRef != nil {
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.AddressGroupRef.ApiVersion,
			Kind:       protoHost.AddressGroupRef.Kind,
			Name:       protoHost.AddressGroupRef.Name,
		}
	}

	// Convert IP list if present
	if len(protoHost.IpList) > 0 {
		host.IpList = make([]models.IPItem, len(protoHost.IpList))
		for i, ipItem := range protoHost.IpList {
			host.IpList[i] = models.IPItem{
				IP: ipItem.Ip,
			}
		}
	}

	// Convert Meta if provided
	if protoHost.Meta != nil {
		host.Meta = models.Meta{
			UID:             protoHost.Meta.Uid,
			ResourceVersion: protoHost.Meta.ResourceVersion,
			Generation:      protoHost.Meta.Generation,
			Labels:          protoHost.Meta.Labels,
			Annotations:     protoHost.Meta.Annotations,
		}
		if protoHost.Meta.CreationTs != nil {
			host.Meta.CreationTS = metav1.NewTime(protoHost.Meta.CreationTs.AsTime())
		}
		if protoHost.Meta.Conditions != nil {
			host.Meta.Conditions = models.ProtoConditionsToK8s(protoHost.Meta.Conditions)
		}
		host.Meta.ObservedGeneration = protoHost.Meta.ObservedGeneration
	}

	return host
}

// convertHostToPB converts domain Host to proto Host
func convertHostToPB(host models.Host) *netguardpb.Host {
	pbHost := &netguardpb.Host{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Uuid: host.UUID,

		// Status fields
		HostNameSync:     host.HostName,
		AddressGroupName: host.AddressGroupName,
		IsBound:          host.IsBound,
	}

	// Convert binding reference if present
	if host.BindingRef != nil {
		pbHost.BindingRef = &netguardpb.ObjectReference{
			ApiVersion: host.BindingRef.APIVersion,
			Kind:       host.BindingRef.Kind,
			Name:       host.BindingRef.Name,
		}
	}

	// Convert address group reference if present
	if host.AddressGroupRef != nil {
		pbHost.AddressGroupRef = &netguardpb.ObjectReference{
			ApiVersion: host.AddressGroupRef.APIVersion,
			Kind:       host.AddressGroupRef.Kind,
			Name:       host.AddressGroupRef.Name,
		}
	}

	// Convert IP list if present
	if len(host.IpList) > 0 {
		pbHost.IpList = make([]*netguardpb.IPItem, len(host.IpList))
		for i, ipItem := range host.IpList {
			pbHost.IpList[i] = &netguardpb.IPItem{
				Ip: ipItem.IP,
			}
		}
	}

	// Populate Meta information
	pbHost.Meta = &netguardpb.Meta{
		Uid:                host.Meta.UID,
		ResourceVersion:    host.Meta.ResourceVersion,
		Generation:         host.Meta.Generation,
		Labels:             host.Meta.Labels,
		Annotations:        host.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(host.Meta.Conditions),
		ObservedGeneration: host.Meta.ObservedGeneration,
	}
	if !host.Meta.CreationTS.IsZero() {
		pbHost.Meta.CreationTs = timestamppb.New(host.Meta.CreationTS.Time)
	}

	return pbHost
}

// convertHostBinding converts proto HostBinding to domain HostBinding
func convertHostBinding(protoBinding *netguardpb.HostBinding) models.HostBinding {
	binding := models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoBinding.SelfRef.Name,
				Namespace: protoBinding.SelfRef.Namespace,
			},
		},
	}

	// Set host reference
	if protoBinding.HostRef != nil {
		binding.HostRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.HostRef.ApiVersion,
				Kind:       protoBinding.HostRef.Kind,
				Name:       protoBinding.HostRef.Name,
			},
			Namespace: protoBinding.HostRef.Namespace,
		}
	}

	// Set address group reference
	if protoBinding.AddressGroupRef != nil {
		binding.AddressGroupRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.AddressGroupRef.ApiVersion,
				Kind:       protoBinding.AddressGroupRef.Kind,
				Name:       protoBinding.AddressGroupRef.Name,
			},
			Namespace: protoBinding.AddressGroupRef.Namespace,
		}
	}

	// Convert Meta if provided
	if protoBinding.Meta != nil {
		binding.Meta = models.Meta{
			UID:             protoBinding.Meta.Uid,
			ResourceVersion: protoBinding.Meta.ResourceVersion,
			Generation:      protoBinding.Meta.Generation,
			Labels:          protoBinding.Meta.Labels,
			Annotations:     protoBinding.Meta.Annotations,
		}
		if protoBinding.Meta.CreationTs != nil {
			binding.Meta.CreationTS = metav1.NewTime(protoBinding.Meta.CreationTs.AsTime())
		}
		if protoBinding.Meta.Conditions != nil {
			binding.Meta.Conditions = models.ProtoConditionsToK8s(protoBinding.Meta.Conditions)
		}
		binding.Meta.ObservedGeneration = protoBinding.Meta.ObservedGeneration
	}

	return binding
}

// convertHostBindingToPB converts domain HostBinding to proto HostBinding
func convertHostBindingToPB(binding models.HostBinding) *netguardpb.HostBinding {
	pbBinding := &netguardpb.HostBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},

		HostRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.HostRef.APIVersion,
			Kind:       binding.HostRef.Kind,
			Name:       binding.HostRef.Name,
			Namespace:  binding.HostRef.Namespace,
		},

		AddressGroupRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
			Namespace:  binding.AddressGroupRef.Namespace,
		},
	}

	// Populate Meta information
	pbBinding.Meta = &netguardpb.Meta{
		Uid:                binding.Meta.UID,
		ResourceVersion:    binding.Meta.ResourceVersion,
		Generation:         binding.Meta.Generation,
		Labels:             binding.Meta.Labels,
		Annotations:        binding.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(binding.Meta.Conditions),
		ObservedGeneration: binding.Meta.ObservedGeneration,
	}
	if !binding.Meta.CreationTS.IsZero() {
		pbBinding.Meta.CreationTs = timestamppb.New(binding.Meta.CreationTS.Time)
	}

	return pbBinding
}
