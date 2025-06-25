package client

import (
	"netguard-pg-backend/internal/domain/models"
	commonpb "netguard-pg-backend/protos/pkg/api/common"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
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
	return models.AddressGroupBinding{
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
}

func convertAddressGroupBindingToProto(binding models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	return &netguardpb.AddressGroupBinding{
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

	return &netguardpb.RuleS2S{
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
	}
	return alias
}

func convertServiceAliasToProto(m models.ServiceAlias) *netguardpb.ServiceAlias {
	return &netguardpb.ServiceAlias{
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
	}
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
	}
	return policy
}

func convertAddressGroupBindingPolicyToProto(m models.AddressGroupBindingPolicy) *netguardpb.AddressGroupBindingPolicy {
	return &netguardpb.AddressGroupBindingPolicy{
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
	}
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

	return proto
}
