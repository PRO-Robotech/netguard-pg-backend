package sync

import (
	"context"

	"netguard-pg-backend/internal/api/netguard/converters"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/client"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Dispatcher handles synchronization operations for different entity types
type Dispatcher struct {
	service *services.NetguardFacade
}

// NewDispatcher creates a new SyncDispatcher
func NewDispatcher(service *services.NetguardFacade) *Dispatcher {
	return &Dispatcher{service: service}
}

// Sync processes a sync request and returns empty response
func (d *Dispatcher) Sync(ctx context.Context, req *netguardpb.SyncReq) (*emptypb.Empty, error) {
	syncOp := d.convertSyncOp(req.SyncOp)
	if err := d.DispatchSync(ctx, syncOp, req.Subject); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// convertSyncOp converts protobuf SyncOp to domain model
func (d *Dispatcher) convertSyncOp(op netguardpb.SyncOp) models.SyncOp {
	switch op {
	case netguardpb.SyncOp_Upsert:
		return models.SyncOpUpsert
	case netguardpb.SyncOp_Delete:
		return models.SyncOpDelete
	default:
		return models.SyncOpNoOp
	}
}

// DispatchSync dispatches sync operations based on the subject type
func (d *Dispatcher) DispatchSync(ctx context.Context, syncOp models.SyncOp, subject interface{}) error {
	switch s := subject.(type) {
	case *netguardpb.SyncReq_Services:
		if s.Services == nil || len(s.Services.Services) == 0 {
			return nil
		}
		services := make([]models.Service, 0, len(s.Services.Services))
		for _, svc := range s.Services.Services {
			services = append(services, converters.ConvertService(svc))
		}
		return d.service.Sync(ctx, syncOp, services)

	case *netguardpb.SyncReq_AddressGroups:
		if s.AddressGroups == nil || len(s.AddressGroups.AddressGroups) == 0 {
			return nil
		}
		addressGroups := make([]models.AddressGroup, 0, len(s.AddressGroups.AddressGroups))
		for _, ag := range s.AddressGroups.AddressGroups {
			addressGroups = append(addressGroups, converters.ConvertAddressGroup(ag))
		}
		return d.service.Sync(ctx, syncOp, addressGroups)

	case *netguardpb.SyncReq_AddressGroupBindings:
		if s.AddressGroupBindings == nil || len(s.AddressGroupBindings.AddressGroupBindings) == 0 {
			return nil
		}
		bindings := make([]models.AddressGroupBinding, 0, len(s.AddressGroupBindings.AddressGroupBindings))
		for _, b := range s.AddressGroupBindings.AddressGroupBindings {
			bindings = append(bindings, converters.ConvertAddressGroupBinding(b))
		}
		return d.service.Sync(ctx, syncOp, bindings)

	case *netguardpb.SyncReq_AddressGroupPortMappings:
		if s.AddressGroupPortMappings == nil || len(s.AddressGroupPortMappings.AddressGroupPortMappings) == 0 {
			return nil
		}
		mappings := make([]models.AddressGroupPortMapping, 0, len(s.AddressGroupPortMappings.AddressGroupPortMappings))
		for _, m := range s.AddressGroupPortMappings.AddressGroupPortMappings {
			mappings = append(mappings, converters.ConvertAddressGroupPortMapping(m))
		}
		return d.service.Sync(ctx, syncOp, mappings)

	case *netguardpb.SyncReq_RuleS2S:
		if s.RuleS2S == nil || len(s.RuleS2S.RuleS2S) == 0 {
			return nil
		}
		rules := make([]models.RuleS2S, 0, len(s.RuleS2S.RuleS2S))
		for _, r := range s.RuleS2S.RuleS2S {
			rules = append(rules, converters.ConvertRuleS2S(r))
		}
		return d.service.Sync(ctx, syncOp, rules)

	case *netguardpb.SyncReq_ServiceAliases:
		if s.ServiceAliases == nil || len(s.ServiceAliases.ServiceAliases) == 0 {
			return nil
		}
		aliases := make([]models.ServiceAlias, 0, len(s.ServiceAliases.ServiceAliases))
		for _, a := range s.ServiceAliases.ServiceAliases {
			aliases = append(aliases, converters.ConvertServiceAlias(a))
		}
		return d.service.Sync(ctx, syncOp, aliases)

	case *netguardpb.SyncReq_IeagagRules:
		if s.IeagagRules == nil || len(s.IeagagRules.IeagagRules) == 0 {
			return nil
		}
		rules := make([]models.IEAgAgRule, 0, len(s.IeagagRules.IeagagRules))
		for _, r := range s.IeagagRules.IeagagRules {
			rule := client.ConvertIEAgAgRuleFromProto(r)
			rules = append(rules, rule)
		}
		return d.service.Sync(ctx, syncOp, rules)

	case *netguardpb.SyncReq_AddressGroupBindingPolicies:
		if s.AddressGroupBindingPolicies == nil || len(s.AddressGroupBindingPolicies.AddressGroupBindingPolicies) == 0 {
			return nil
		}
		policies := make([]models.AddressGroupBindingPolicy, 0, len(s.AddressGroupBindingPolicies.AddressGroupBindingPolicies))
		for _, p := range s.AddressGroupBindingPolicies.AddressGroupBindingPolicies {
			policies = append(policies, converters.ConvertAddressGroupBindingPolicy(p))
		}
		return d.service.Sync(ctx, syncOp, policies)

	case *netguardpb.SyncReq_Networks:
		if s.Networks == nil || len(s.Networks.Networks) == 0 {
			return nil
		}
		networks := make([]models.Network, 0, len(s.Networks.Networks))
		for _, n := range s.Networks.Networks {
			networks = append(networks, converters.ConvertNetwork(n))
		}
		return d.service.Sync(ctx, syncOp, networks)

	case *netguardpb.SyncReq_NetworkBindings:
		if s.NetworkBindings == nil || len(s.NetworkBindings.NetworkBindings) == 0 {
			return nil
		}
		bindings := make([]models.NetworkBinding, 0, len(s.NetworkBindings.NetworkBindings))
		for _, b := range s.NetworkBindings.NetworkBindings {
			bindings = append(bindings, converters.ConvertNetworkBinding(b))
		}
		return d.service.Sync(ctx, syncOp, bindings)

	case *netguardpb.SyncReq_Hosts:
		if s.Hosts == nil || len(s.Hosts.Hosts) == 0 {
			return nil
		}
		hosts := make([]models.Host, 0, len(s.Hosts.Hosts))
		for _, h := range s.Hosts.Hosts {
			hosts = append(hosts, converters.ConvertHost(h))
		}
		return d.service.Sync(ctx, syncOp, hosts)

	case *netguardpb.SyncReq_HostBindings:
		if s.HostBindings == nil || len(s.HostBindings.HostBindings) == 0 {
			return nil
		}
		bindings := make([]models.HostBinding, 0, len(s.HostBindings.HostBindings))
		for _, b := range s.HostBindings.HostBindings {
			bindings = append(bindings, converters.ConvertHostBinding(b))
		}
		return d.service.Sync(ctx, syncOp, bindings)

	default:
		return errors.New("subject not specified")
	}
}
