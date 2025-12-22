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
		Comment:     svc.Comment,
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

	// Convert XSvcSvcRules (READ-ONLY metadata)
	if svc.XSvcsvcRules != nil {
		domainRules := &models.XSvcSvcRules{}
		if len(svc.XSvcsvcRules.AsServiceFrom) > 0 {
			domainRules.AsServiceFrom = make([]v1beta1.NamespacedObjectReference, len(svc.XSvcsvcRules.AsServiceFrom))
			for i, ref := range svc.XSvcsvcRules.AsServiceFrom {
				domainRules.AsServiceFrom[i] = NewNamespacedObjectReference(
					KindSvcSvcRule,
					ref.GetName(),
					ref.GetNamespace(),
				)
			}
		}
		if len(svc.XSvcsvcRules.AsServiceTo) > 0 {
			domainRules.AsServiceTo = make([]v1beta1.NamespacedObjectReference, len(svc.XSvcsvcRules.AsServiceTo))
			for i, ref := range svc.XSvcsvcRules.AsServiceTo {
				domainRules.AsServiceTo[i] = NewNamespacedObjectReference(
					KindSvcSvcRule,
					ref.GetName(),
					ref.GetNamespace(),
				)
			}
		}
		result.XSvcSvcRules = domainRules
	}

	// Convert XSvcFqdnRules (READ-ONLY metadata)
	if svc.XSvcfqdnRules != nil {
		domainFqdnRules := &models.XSvcFqdnRules{}
		if len(svc.XSvcfqdnRules.Rules) > 0 {
			domainFqdnRules.Rules = make([]models.ResourceIdentifier, len(svc.XSvcfqdnRules.Rules))
			for i, ref := range svc.XSvcfqdnRules.Rules {
				domainFqdnRules.Rules[i] = models.NewResourceIdentifier(ref.GetName(), models.WithNamespace(ref.GetNamespace()))
			}
		}
		result.XSvcFqdnRules = domainFqdnRules
	}

	// Convert XIECidrSvcRules (READ-ONLY metadata)
	if svc.XIecidrsvcRules != nil {
		domainIECidrRules := &models.XIECidrSvcRules{}
		if len(svc.XIecidrsvcRules.Rules) > 0 {
			domainIECidrRules.Rules = make([]models.ResourceIdentifier, len(svc.XIecidrsvcRules.Rules))
			for i, ref := range svc.XIecidrsvcRules.Rules {
				domainIECidrRules.Rules[i] = models.NewResourceIdentifier(ref.GetName(), models.WithNamespace(ref.GetNamespace()))
			}
		}
		result.XIECidrSvcRules = domainIECidrRules
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
		Comment:     svc.Comment,
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

	// Convert XSvcSvcRules (READ-ONLY field populated by PostgreSQL triggers)
	if svc.XSvcSvcRules != nil {
		result.XSvcsvcRules = &netguardpb.XSvcSvcRules{}

		// Convert AsServiceFrom references
		if len(svc.XSvcSvcRules.AsServiceFrom) > 0 {
			result.XSvcsvcRules.AsServiceFrom = make([]*netguardpb.ResourceIdentifier, len(svc.XSvcSvcRules.AsServiceFrom))
			for i, ref := range svc.XSvcSvcRules.AsServiceFrom {
				result.XSvcsvcRules.AsServiceFrom[i] = &netguardpb.ResourceIdentifier{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				}
			}
		}

		// Convert AsServiceTo references
		if len(svc.XSvcSvcRules.AsServiceTo) > 0 {
			result.XSvcsvcRules.AsServiceTo = make([]*netguardpb.ResourceIdentifier, len(svc.XSvcSvcRules.AsServiceTo))
			for i, ref := range svc.XSvcSvcRules.AsServiceTo {
				result.XSvcsvcRules.AsServiceTo[i] = &netguardpb.ResourceIdentifier{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				}
			}
		}
	}
	if svc.XSvcFqdnRules != nil {
		result.XSvcfqdnRules = &netguardpb.XSvcFqdnRules{}
		if len(svc.XSvcFqdnRules.Rules) > 0 {
			result.XSvcfqdnRules.Rules = make([]*netguardpb.ResourceIdentifier, len(svc.XSvcFqdnRules.Rules))
			for i, ref := range svc.XSvcFqdnRules.Rules {
				result.XSvcfqdnRules.Rules[i] = &netguardpb.ResourceIdentifier{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				}
			}
		}
	}

	// Convert XIECidrSvcRules (READ-ONLY field populated by PostgreSQL triggers)
	if svc.XIECidrSvcRules != nil {
		result.XIecidrsvcRules = &netguardpb.XIECidrSvcRules{}
		if len(svc.XIECidrSvcRules.Rules) > 0 {
			result.XIecidrsvcRules.Rules = make([]*netguardpb.ResourceIdentifier, len(svc.XIECidrSvcRules.Rules))
			for i, ref := range svc.XIECidrSvcRules.Rules {
				result.XIecidrsvcRules.Rules[i] = &netguardpb.ResourceIdentifier{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				}
			}
		}
	}

	return result
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
