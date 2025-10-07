package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertService converts protobuf Service to domain model
func ConvertService(svc *netguardpb.Service) models.Service {
	result := models.Service{
		SelfRef:     models.NewSelfRef(GetSelfRef(svc.GetSelfRef())),
		Description: svc.Description,
		Meta:        ConvertMeta(svc.Meta),
	}

	// Convert ingress ports
	for _, p := range svc.IngressPorts {
		result.IngressPorts = append(result.IngressPorts, models.IngressPort{
			Protocol:    ConvertTransportFromPB(p.Protocol),
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

// ConvertServiceToPB converts domain Service to protobuf
func ConvertServiceToPB(svc models.Service) *netguardpb.Service {
	result := &netguardpb.Service{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      svc.ResourceIdentifier.Name,
			Namespace: svc.ResourceIdentifier.Namespace,
		},
		Description: svc.Description,
		Meta:        ConvertMetaToPB(svc.Meta),
	}

	// Convert ingress ports
	for _, p := range svc.IngressPorts {
		result.IngressPorts = append(result.IngressPorts, &netguardpb.IngressPort{
			Protocol:    ConvertTransportToPB(p.Protocol),
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

// ConvertServiceAlias converts protobuf ServiceAlias to domain model
func ConvertServiceAlias(a *netguardpb.ServiceAlias) models.ServiceAlias {
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
			SelfRef: models.NewSelfRef(GetSelfRef(a.GetSelfRef())),
			Meta:    ConvertMeta(a.Meta),
		}
	}

	return models.ServiceAlias{
		SelfRef:    models.NewSelfRef(GetSelfRef(a.GetSelfRef())),
		ServiceRef: models.NewServiceRef(serviceName, models.WithNamespace(serviceNamespace)),
		Meta:       ConvertMeta(a.Meta),
	}
}

// ConvertServiceAliasToPB converts domain ServiceAlias to protobuf
func ConvertServiceAliasToPB(a models.ServiceAlias) *netguardpb.ServiceAlias {
	return &netguardpb.ServiceAlias{
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
		Meta: ConvertMetaToPB(a.Meta),
	}
}

// Helper functions for AddressGroup registration source conversion
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
