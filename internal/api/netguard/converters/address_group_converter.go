package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertAddressGroup converts protobuf AddressGroup to domain model
func ConvertAddressGroup(ag *netguardpb.AddressGroup) models.AddressGroup {
	result := models.AddressGroup{
		SelfRef:       models.NewSelfRef(GetSelfRef(ag.GetSelfRef())),
		DefaultAction: ConvertActionFromPB(ag.DefaultAction),
		Logs:          ag.Logs,
		Trace:         ag.Trace,
		Meta:          ConvertMeta(ag.Meta),
	}

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

	// Convert AggregatedHosts
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

	return result
}

// ConvertAddressGroupToPB converts domain AddressGroup to protobuf
func ConvertAddressGroupToPB(ag models.AddressGroup) *netguardpb.AddressGroup {
	// Convert RuleAction using dedicated converter function
	defaultAction := ConvertActionToPB(ag.DefaultAction)

	result := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      ag.ResourceIdentifier.Name,
			Namespace: ag.ResourceIdentifier.Namespace,
		},
		DefaultAction:    defaultAction,
		Logs:             ag.Logs,
		Trace:            ag.Trace,
		AddressGroupName: ag.AddressGroupName,
		Meta:             ConvertMetaToPB(ag.Meta),
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

	// Convert AggregatedHosts field
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

// ConvertAddressGroupBinding converts protobuf AddressGroupBinding to domain model
func ConvertAddressGroupBinding(b *netguardpb.AddressGroupBinding) models.AddressGroupBinding {
	result := models.AddressGroupBinding{
		SelfRef: models.NewSelfRef(GetSelfRef(b.GetSelfRef())),
		Meta:    ConvertMeta(b.Meta),
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

	return result
}

// ConvertAddressGroupBindingToPB converts domain AddressGroupBinding to protobuf
func ConvertAddressGroupBindingToPB(b models.AddressGroupBinding) *netguardpb.AddressGroupBinding {
	return &netguardpb.AddressGroupBinding{
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
		Meta: ConvertMetaToPB(b.Meta),
	}
}

// ConvertAddressGroupPortMapping converts protobuf AddressGroupPortMapping to domain model
func ConvertAddressGroupPortMapping(m *netguardpb.AddressGroupPortMapping) models.AddressGroupPortMapping {
	result := models.AddressGroupPortMapping{
		SelfRef:     models.NewSelfRef(GetSelfRef(m.GetSelfRef())),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{},
		Meta:        ConvertMeta(m.Meta),
	}

	// Convert access ports
	for _, ap := range m.AccessPorts {
		spr := models.NewServiceRef(
			ap.Identifier.Name,
			models.WithNamespace(ap.GetIdentifier().GetNamespace()),
		)
		ports := make(models.ProtocolPorts)

		// Convert ports
		for pType, ranges := range ap.Ports.Ports {
			portRanges := make([]models.PortRange, 0, len(ranges.Ranges))
			for _, r := range ranges.Ranges {
				portRanges = append(portRanges, models.PortRange{
					Start: int(r.Start),
					End:   int(r.End),
				})
			}
			ports[models.TransportProtocol(pType)] = portRanges
		}

		result.AccessPorts[spr] = models.ServicePorts{Ports: ports}
	}

	return result
}

// ConvertAddressGroupPortMappingToPB converts domain AddressGroupPortMapping to protobuf
func ConvertAddressGroupPortMappingToPB(m models.AddressGroupPortMapping) *netguardpb.AddressGroupPortMapping {
	result := &netguardpb.AddressGroupPortMapping{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		Meta: ConvertMetaToPB(m.Meta),
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

// ConvertAddressGroupBindingPolicy converts protobuf AddressGroupBindingPolicy to domain model
func ConvertAddressGroupBindingPolicy(policy *netguardpb.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	result := models.AddressGroupBindingPolicy{
		SelfRef: models.NewSelfRef(GetSelfRef(policy.GetSelfRef())),
		Meta:    ConvertMeta(policy.Meta),
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

	return result
}

// ConvertAddressGroupBindingPolicyToPB converts domain AddressGroupBindingPolicy to protobuf
func ConvertAddressGroupBindingPolicyToPB(policy models.AddressGroupBindingPolicy) *netguardpb.AddressGroupBindingPolicy {
	return &netguardpb.AddressGroupBindingPolicy{
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
		Meta: ConvertMetaToPB(policy.Meta),
	}
}

// Helper functions for host registration source conversion
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
