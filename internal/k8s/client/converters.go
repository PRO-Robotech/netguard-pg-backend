package client

import (
	"netguard-pg-backend/internal/domain/models"
	commonpb "netguard-pg-backend/protos/pkg/api/common"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service конверторы
func convertServiceFromProto(protoSvc *netguardpb.Service) models.Service {
	service := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoSvc.SelfRef.Name,
				models.WithNamespace(protoSvc.SelfRef.Namespace),
			),
		},
		Description: protoSvc.Description,
	}

	// meta
	if protoSvc.Meta != nil {
		service.Meta = models.Meta{
			UID:             protoSvc.Meta.Uid,
			ResourceVersion: protoSvc.Meta.ResourceVersion,
			Generation:      protoSvc.Meta.Generation,
			Labels:          protoSvc.Meta.Labels,
			Annotations:     protoSvc.Meta.Annotations,
		}
		if protoSvc.Meta.CreationTs != nil {
			service.Meta.CreationTS = metav1.NewTime(protoSvc.Meta.CreationTs.AsTime())
		}
	}

	// Конвертация IngressPorts
	for _, port := range protoSvc.IngressPorts {
		var protocol models.TransportProtocol
		switch port.Protocol {
		case commonpb.Networks_NetIP_TCP:
			protocol = models.TCP
		case commonpb.Networks_NetIP_UDP:
			protocol = models.UDP
		default:
			protocol = models.TCP // default
		}

		service.IngressPorts = append(service.IngressPorts, models.IngressPort{
			Protocol:    protocol,
			Port:        port.Port,
			Description: port.Description,
		})
	}

	// Конвертация AddressGroups
	for _, agRef := range protoSvc.AddressGroups {
		service.AddressGroups = append(service.AddressGroups, models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				agRef.Identifier.Name,
				models.WithNamespace(agRef.Identifier.Namespace),
			),
		})
	}

	return service
}

func convertServiceToProto(service models.Service) *netguardpb.Service {
	protoSvc := &netguardpb.Service{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      service.ResourceIdentifier.Name,
			Namespace: service.ResourceIdentifier.Namespace,
		},
		Description: service.Description,
		Meta: &netguardpb.Meta{
			Uid:             service.Meta.UID,
			ResourceVersion: service.Meta.ResourceVersion,
			Generation:      service.Meta.Generation,
			Labels:          service.Meta.Labels,
			Annotations:     service.Meta.Annotations,
		},
	}

	if !service.Meta.CreationTS.IsZero() {
		protoSvc.Meta.CreationTs = timestamppb.New(service.Meta.CreationTS.Time)
	}

	// Конвертация IngressPorts
	for _, port := range service.IngressPorts {
		var protocol commonpb.Networks_NetIP_Transport
		switch port.Protocol {
		case models.TCP:
			protocol = commonpb.Networks_NetIP_TCP
		case models.UDP:
			protocol = commonpb.Networks_NetIP_UDP
		default:
			protocol = commonpb.Networks_NetIP_TCP // default
		}

		protoSvc.IngressPorts = append(protoSvc.IngressPorts, &netguardpb.IngressPort{
			Protocol:    protocol,
			Port:        port.Port,
			Description: port.Description,
		})
	}

	// Конвертация AddressGroups
	for _, agRef := range service.AddressGroups {
		protoSvc.AddressGroups = append(protoSvc.AddressGroups, &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      agRef.ResourceIdentifier.Name,
				Namespace: agRef.ResourceIdentifier.Namespace,
			},
		})
	}

	return protoSvc
}

// AddressGroup конверторы
func convertAddressGroupFromProto(protoAG *netguardpb.AddressGroup) models.AddressGroup {
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoAG.SelfRef.Name,
				models.WithNamespace(protoAG.SelfRef.Namespace),
			),
		},
		Description: protoAG.Description,
		Addresses:   protoAG.Addresses,
		Meta:        models.Meta{},
	}

	if protoAG.Meta != nil {
		addressGroup.Meta = models.Meta{
			UID:             protoAG.Meta.Uid,
			ResourceVersion: protoAG.Meta.ResourceVersion,
			Generation:      protoAG.Meta.Generation,
		}
		if protoAG.Meta.CreationTs != nil {
			addressGroup.Meta.CreationTS = metav1.NewTime(protoAG.Meta.CreationTs.AsTime())
		}
		addressGroup.Meta.Labels = protoAG.Meta.Labels
		addressGroup.Meta.Annotations = protoAG.Meta.Annotations
	}

	// Конвертация Services
	for _, svcRef := range protoAG.Services {
		addressGroup.Services = append(addressGroup.Services, models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				svcRef.Identifier.Name,
				models.WithNamespace(svcRef.Identifier.Namespace),
			),
		})
	}

	return addressGroup
}

func convertAddressGroupToProto(addressGroup models.AddressGroup) *netguardpb.AddressGroup {
	protoAG := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      addressGroup.ResourceIdentifier.Name,
			Namespace: addressGroup.ResourceIdentifier.Namespace,
		},
		Description: addressGroup.Description,
		Addresses:   addressGroup.Addresses,
		Meta: &netguardpb.Meta{
			Uid:             addressGroup.Meta.UID,
			ResourceVersion: addressGroup.Meta.ResourceVersion,
			Generation:      addressGroup.Meta.Generation,
			Labels:          addressGroup.Meta.Labels,
			Annotations:     addressGroup.Meta.Annotations,
		},
	}

	if !addressGroup.Meta.CreationTS.IsZero() {
		protoAG.Meta.CreationTs = timestamppb.New(addressGroup.Meta.CreationTS.Time)
	}

	// Конвертация Services
	for _, svcRef := range addressGroup.Services {
		protoAG.Services = append(protoAG.Services, &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      svcRef.ResourceIdentifier.Name,
				Namespace: svcRef.ResourceIdentifier.Namespace,
			},
		})
	}

	return protoAG
}

// AddressGroupBinding конверторы
func convertAddressGroupBindingFromProto(protoBinding *netguardpb.AddressGroupBinding) models.AddressGroupBinding {
	binding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoBinding.SelfRef.Name,
				models.WithNamespace(protoBinding.SelfRef.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoBinding.ServiceRef.Identifier.Name,
				models.WithNamespace(protoBinding.ServiceRef.Identifier.Namespace),
			),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoBinding.AddressGroupRef.Identifier.Name,
				models.WithNamespace(protoBinding.AddressGroupRef.Identifier.Namespace),
			),
		},
	}

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
	}

	return binding
}

func convertAddressGroupBindingToProto(binding models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	protoBinding := &netguardpb.AddressGroupBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.ResourceIdentifier.Name,
			Namespace: binding.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      binding.ServiceRef.ResourceIdentifier.Name,
				Namespace: binding.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      binding.AddressGroupRef.ResourceIdentifier.Name,
				Namespace: binding.AddressGroupRef.ResourceIdentifier.Namespace,
			},
		},
	}

	if !binding.Meta.CreationTS.IsZero() {
		protoBinding.Meta = &netguardpb.Meta{
			Uid:             binding.Meta.UID,
			ResourceVersion: binding.Meta.ResourceVersion,
			Generation:      binding.Meta.Generation,
			Labels:          binding.Meta.Labels,
			Annotations:     binding.Meta.Annotations,
		}
		protoBinding.Meta.CreationTs = timestamppb.New(binding.Meta.CreationTS.Time)
	}

	return protoBinding
}

// AddressGroupPortMapping конверторы
func convertAddressGroupPortMappingFromProto(proto *netguardpb.AddressGroupPortMapping) models.AddressGroupPortMapping {
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Конвертация AccessPorts из []*ServicePortsRef в map[ServiceRef]ServicePorts
	for _, protoSPR := range proto.AccessPorts {
		serviceRef := models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoSPR.Identifier.Name,
				models.WithNamespace(protoSPR.Identifier.Namespace),
			),
		}

		servicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		// Конвертация ProtocolPorts
		if protoSPR.Ports != nil {
			for protoName, protoRanges := range protoSPR.Ports.Ports {
				protocol := models.TransportProtocol(protoName)
				var ranges []models.PortRange

				for _, protoRange := range protoRanges.Ranges {
					ranges = append(ranges, models.PortRange{
						Start: int(protoRange.Start),
						End:   int(protoRange.End),
					})
				}

				servicePorts.Ports[protocol] = ranges
			}
		}

		mapping.AccessPorts[serviceRef] = servicePorts
	}

	if proto.Meta != nil {
		mapping.Meta = models.Meta{
			UID:             proto.Meta.Uid,
			ResourceVersion: proto.Meta.ResourceVersion,
			Generation:      proto.Meta.Generation,
			Labels:          proto.Meta.Labels,
			Annotations:     proto.Meta.Annotations,
		}
		if proto.Meta.CreationTs != nil {
			mapping.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return mapping
}

func convertAddressGroupPortMappingToProto(m models.AddressGroupPortMapping) *netguardpb.AddressGroupPortMapping {
	proto := &netguardpb.AddressGroupPortMapping{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
	}

	// Конвертация AccessPorts из map[ServiceRef]ServicePorts в []*ServicePortsRef
	for serviceRef, servicePorts := range m.AccessPorts {
		protoSPR := &netguardpb.ServicePortsRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      serviceRef.ResourceIdentifier.Name,
				Namespace: serviceRef.ResourceIdentifier.Namespace,
			},
			Ports: &netguardpb.ProtocolPorts{
				Ports: make(map[string]*netguardpb.PortRanges),
			},
		}

		// Конвертация ProtocolPorts
		for protocol, ranges := range servicePorts.Ports {
			var protoRanges []*netguardpb.PortRange
			for _, r := range ranges {
				protoRanges = append(protoRanges, &netguardpb.PortRange{
					Start: int32(r.Start),
					End:   int32(r.End),
				})
			}
			protoSPR.Ports.Ports[string(protocol)] = &netguardpb.PortRanges{
				Ranges: protoRanges,
			}
		}

		proto.AccessPorts = append(proto.AccessPorts, protoSPR)
	}

	if !m.Meta.CreationTS.IsZero() {
		proto.Meta = &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		}
		proto.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	return proto
}

// RuleS2S конверторы
func convertRuleS2SFromProto(proto *netguardpb.RuleS2S) models.RuleS2S {
	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		Traffic: models.Traffic(proto.Traffic.String()),
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.ServiceLocalRef.Identifier.Name,
				models.WithNamespace(proto.ServiceLocalRef.Identifier.Namespace),
			),
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.ServiceRef.Identifier.Name,
				models.WithNamespace(proto.ServiceRef.Identifier.Namespace),
			),
		},
	}

	if proto.Meta != nil {
		rule.Meta = models.Meta{
			UID:             proto.Meta.Uid,
			ResourceVersion: proto.Meta.ResourceVersion,
			Generation:      proto.Meta.Generation,
			Labels:          proto.Meta.Labels,
			Annotations:     proto.Meta.Annotations,
		}
		if proto.Meta.CreationTs != nil {
			rule.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return rule
}

func convertRuleS2SToProto(m models.RuleS2S) *netguardpb.RuleS2S {
	// Конвертация Traffic string в protobuf enum
	var traffic commonpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = commonpb.Traffic_Ingress
	case models.EGRESS:
		traffic = commonpb.Traffic_Egress
	default:
		traffic = commonpb.Traffic_Ingress // default
	}

	proto := &netguardpb.RuleS2S{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		Traffic: traffic,
		ServiceLocalRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceLocalRef.ResourceIdentifier.Name,
				Namespace: m.ServiceLocalRef.ResourceIdentifier.Namespace,
			},
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceRef.ResourceIdentifier.Name,
				Namespace: m.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
	}

	if !m.Meta.CreationTS.IsZero() {
		proto.Meta = &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		}
		proto.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	return proto
}

// ServiceAlias конверторы
func convertServiceAliasFromProto(proto *netguardpb.ServiceAlias) models.ServiceAlias {
	alias := models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.ServiceRef.Identifier.Name,
				models.WithNamespace(proto.ServiceRef.Identifier.Namespace),
			),
		},
		Meta: models.Meta{},
	}

	if proto.Meta != nil {
		alias.Meta = models.Meta{
			UID:             proto.Meta.Uid,
			ResourceVersion: proto.Meta.ResourceVersion,
			Generation:      proto.Meta.Generation,
			Labels:          proto.Meta.Labels,
			Annotations:     proto.Meta.Annotations,
		}
		if proto.Meta.CreationTs != nil {
			alias.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return alias
}

func convertServiceAliasToProto(m models.ServiceAlias) *netguardpb.ServiceAlias {
	protoAlias := &netguardpb.ServiceAlias{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceRef.ResourceIdentifier.Name,
				Namespace: m.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
		Meta: &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		},
	}
	if !m.Meta.CreationTS.IsZero() {
		protoAlias.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}
	return protoAlias
}

// AddressGroupBindingPolicy конверторы
func convertAddressGroupBindingPolicyFromProto(proto *netguardpb.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	policy := models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.ServiceRef.Identifier.Name,
				models.WithNamespace(proto.ServiceRef.Identifier.Namespace),
			),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.AddressGroupRef.Identifier.Name,
				models.WithNamespace(proto.AddressGroupRef.Identifier.Namespace),
			),
		},
		Meta: models.Meta{},
	}
	if proto.Meta != nil {
		policy.Meta = models.Meta{
			UID:             proto.Meta.Uid,
			ResourceVersion: proto.Meta.ResourceVersion,
			Generation:      proto.Meta.Generation,
			Labels:          proto.Meta.Labels,
			Annotations:     proto.Meta.Annotations,
		}
		if proto.Meta.CreationTs != nil {
			policy.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}
	return policy
}

func convertAddressGroupBindingPolicyToProto(m models.AddressGroupBindingPolicy) *netguardpb.AddressGroupBindingPolicy {
	protoPol := &netguardpb.AddressGroupBindingPolicy{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceRef.ResourceIdentifier.Name,
				Namespace: m.ServiceRef.ResourceIdentifier.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.AddressGroupRef.ResourceIdentifier.Name,
				Namespace: m.AddressGroupRef.ResourceIdentifier.Namespace,
			},
		},
		Meta: &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		},
	}
	if !m.Meta.CreationTS.IsZero() {
		protoPol.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}
	return protoPol
}

// IEAgAgRule конверторы
func convertIEAgAgRuleFromProto(proto *netguardpb.IEAgAgRule) models.IEAgAgRule {
	rule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		Transport: models.TransportProtocol(proto.Transport.String()),
		Traffic:   models.Traffic(proto.Traffic.String()),
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.AddressGroupLocal.Identifier.Name,
				models.WithNamespace(proto.AddressGroupLocal.Identifier.Namespace),
			),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.AddressGroup.Identifier.Name,
				models.WithNamespace(proto.AddressGroup.Identifier.Namespace),
			),
		},
		Action:   models.RuleAction(proto.Action),
		Logs:     proto.Logs,
		Priority: proto.Priority,
	}

	// Конвертация Ports
	for _, portSpec := range proto.Ports {
		rule.Ports = append(rule.Ports, models.PortSpec{
			Source:      portSpec.Source,
			Destination: portSpec.Destination,
		})
	}

	if proto.Meta != nil {
		rule.Meta = models.Meta{
			UID:             proto.Meta.Uid,
			ResourceVersion: proto.Meta.ResourceVersion,
			Generation:      proto.Meta.Generation,
			Labels:          proto.Meta.Labels,
			Annotations:     proto.Meta.Annotations,
		}
		if proto.Meta.CreationTs != nil {
			rule.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return rule
}

func convertIEAgAgRuleToProto(m models.IEAgAgRule) *netguardpb.IEAgAgRule {
	// Конвертация Transport string в protobuf enum
	var transport commonpb.Networks_NetIP_Transport
	switch m.Transport {
	case models.TCP:
		transport = commonpb.Networks_NetIP_TCP
	case models.UDP:
		transport = commonpb.Networks_NetIP_UDP
	default:
		transport = commonpb.Networks_NetIP_TCP // default
	}

	// Конвертация Traffic string в protobuf enum
	var traffic commonpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = commonpb.Traffic_Ingress
	case models.EGRESS:
		traffic = commonpb.Traffic_Egress
	default:
		traffic = commonpb.Traffic_Ingress // default
	}

	proto := &netguardpb.IEAgAgRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		Transport: transport,
		Traffic:   traffic,
		AddressGroupLocal: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.AddressGroupLocal.ResourceIdentifier.Name,
				Namespace: m.AddressGroupLocal.ResourceIdentifier.Namespace,
			},
		},
		AddressGroup: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.AddressGroup.ResourceIdentifier.Name,
				Namespace: m.AddressGroup.ResourceIdentifier.Namespace,
			},
		},
		Action:   string(m.Action),
		Logs:     m.Logs,
		Priority: m.Priority,
	}

	// Конвертация Ports
	for _, portSpec := range m.Ports {
		proto.Ports = append(proto.Ports, &netguardpb.PortSpec{
			Source:      portSpec.Source,
			Destination: portSpec.Destination,
		})
	}

	if !m.Meta.CreationTS.IsZero() {
		proto.Meta = &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
		}
		proto.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	return proto
}
