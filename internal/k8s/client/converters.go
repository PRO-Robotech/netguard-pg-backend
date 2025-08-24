package client

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	// commonpb "github.com/H-BF/protos/pkg/api/common" - replaced with local types
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ManagedFields conversion helpers
// These functions convert between protobuf ManagedFieldsEntry and k8s ManagedFieldsEntry

// convertManagedFieldsFromProto converts protobuf ManagedFieldsEntry slice to k8s ManagedFieldsEntry slice
func convertManagedFieldsFromProto(protoFields []*netguardpb.ManagedFieldsEntry) []metav1.ManagedFieldsEntry {
	if len(protoFields) == 0 {
		return nil
	}

	managedFields := make([]metav1.ManagedFieldsEntry, len(protoFields))
	for i, protoField := range protoFields {
		managedFields[i] = metav1.ManagedFieldsEntry{
			Manager:     protoField.Manager,
			Operation:   metav1.ManagedFieldsOperationType(protoField.Operation),
			APIVersion:  protoField.ApiVersion,
			FieldsType:  protoField.FieldsType,
			Subresource: protoField.Subresource,
		}

		// Convert timestamp
		if protoField.Time != nil {
			managedFields[i].Time = &metav1.Time{
				Time: protoField.Time.AsTime(),
			}
		}

		// Convert FieldsV1
		if len(protoField.FieldsV1) > 0 {
			managedFields[i].FieldsV1 = &metav1.FieldsV1{
				Raw: protoField.FieldsV1,
			}
		}
	}

	return managedFields
}

// convertManagedFieldsToProto converts k8s ManagedFieldsEntry slice to protobuf ManagedFieldsEntry slice
func convertManagedFieldsToProto(managedFields []metav1.ManagedFieldsEntry) []*netguardpb.ManagedFieldsEntry {
	if len(managedFields) == 0 {
		return nil
	}

	protoFields := make([]*netguardpb.ManagedFieldsEntry, len(managedFields))
	for i, field := range managedFields {
		protoFields[i] = &netguardpb.ManagedFieldsEntry{
			Manager:     field.Manager,
			Operation:   string(field.Operation),
			ApiVersion:  field.APIVersion,
			FieldsType:  field.FieldsType,
			Subresource: field.Subresource,
		}

		// Convert timestamp
		if field.Time != nil {
			protoFields[i].Time = timestamppb.New(field.Time.Time)
		}

		// Convert FieldsV1
		if field.FieldsV1 != nil {
			protoFields[i].FieldsV1 = field.FieldsV1.Raw
		}
	}

	return protoFields
}

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
			UID:                protoSvc.Meta.Uid,
			ResourceVersion:    protoSvc.Meta.ResourceVersion,
			Generation:         protoSvc.Meta.Generation,
			Labels:             protoSvc.Meta.Labels,
			Annotations:        protoSvc.Meta.Annotations,
			GeneratedName:      protoSvc.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(protoSvc.Meta.Conditions),
			ObservedGeneration: protoSvc.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(protoSvc.Meta.ManagedFields),
		}
		if protoSvc.Meta.CreationTs != nil {
			service.Meta.CreationTS = metav1.NewTime(protoSvc.Meta.CreationTs.AsTime())
		}
	}

	// Конвертация IngressPorts
	for _, port := range protoSvc.IngressPorts {
		var protocol models.TransportProtocol
		switch port.Protocol {
		case netguardpb.Networks_NetIP_TCP:
			protocol = models.TCP
		case netguardpb.Networks_NetIP_UDP:
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
		service.AddressGroups = append(service.AddressGroups, models.NewAddressGroupRef(
			agRef.Identifier.Name,
			models.WithNamespace(agRef.Identifier.Namespace),
		))
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
			GeneratedName:   service.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(service.Meta.ManagedFields),
		},
	}

	if !service.Meta.CreationTS.IsZero() {
		protoSvc.Meta.CreationTs = timestamppb.New(service.Meta.CreationTS.Time)
	}

	// Конвертация IngressPorts
	for _, port := range service.IngressPorts {
		var protocol netguardpb.Networks_NetIP_Transport
		switch port.Protocol {
		case models.TCP:
			protocol = netguardpb.Networks_NetIP_TCP
		case models.UDP:
			protocol = netguardpb.Networks_NetIP_UDP
		default:
			protocol = netguardpb.Networks_NetIP_TCP // default
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
				Name:      agRef.Name,
				Namespace: agRef.Namespace,
			},
		})
	}

	return protoSvc
}

// AddressGroup конверторы
func convertAddressGroupFromProto(protoAG *netguardpb.AddressGroup) models.AddressGroup {
	// Конвертация RuleAction protobuf enum в string
	var defaultAction models.RuleAction
	switch protoAG.DefaultAction {
	case netguardpb.RuleAction_ACCEPT:
		defaultAction = models.ActionAccept
	case netguardpb.RuleAction_DROP:
		defaultAction = models.ActionDrop
	default:
		defaultAction = models.ActionDrop // default
	}

	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoAG.SelfRef.Name,
				models.WithNamespace(protoAG.SelfRef.Namespace),
			),
		},
		DefaultAction: defaultAction,
		Logs:          protoAG.Logs,
		Trace:         protoAG.Trace,
	}

	// meta
	if protoAG.Meta != nil {
		addressGroup.Meta = models.Meta{
			UID:                protoAG.Meta.Uid,
			ResourceVersion:    protoAG.Meta.ResourceVersion,
			Generation:         protoAG.Meta.Generation,
			Labels:             protoAG.Meta.Labels,
			Annotations:        protoAG.Meta.Annotations,
			GeneratedName:      protoAG.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(protoAG.Meta.Conditions),
			ObservedGeneration: protoAG.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(protoAG.Meta.ManagedFields),
		}
		if protoAG.Meta.CreationTs != nil {
			addressGroup.Meta.CreationTS = metav1.NewTime(protoAG.Meta.CreationTs.AsTime())
		}
	}

	// Convert Networks list
	for _, protoNetworkItem := range protoAG.Networks {
		addressGroup.Networks = append(addressGroup.Networks, models.NetworkItem{
			Name:       protoNetworkItem.Name,
			CIDR:       protoNetworkItem.Cidr,
			ApiVersion: protoNetworkItem.ApiVersion,
			Kind:       protoNetworkItem.Kind,
			Namespace:  protoNetworkItem.Namespace,
		})
	}

	return addressGroup
}

func convertAddressGroupToProto(addressGroup models.AddressGroup) *netguardpb.AddressGroup {
	// Конвертация RuleAction string в protobuf enum
	var defaultAction netguardpb.RuleAction
	switch addressGroup.DefaultAction {
	case models.ActionAccept:
		defaultAction = netguardpb.RuleAction_ACCEPT
	case models.ActionDrop:
		defaultAction = netguardpb.RuleAction_DROP
	default:
		defaultAction = netguardpb.RuleAction_DROP // default
	}

	protoAG := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      addressGroup.ResourceIdentifier.Name,
			Namespace: addressGroup.ResourceIdentifier.Namespace,
		},
		DefaultAction: defaultAction,
		Logs:          addressGroup.Logs,
		Trace:         addressGroup.Trace,
		Meta: &netguardpb.Meta{
			Uid:             addressGroup.Meta.UID,
			ResourceVersion: addressGroup.Meta.ResourceVersion,
			Generation:      addressGroup.Meta.Generation,
			Labels:          addressGroup.Meta.Labels,
			Annotations:     addressGroup.Meta.Annotations,
			GeneratedName:   addressGroup.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(addressGroup.Meta.ManagedFields),
		},
	}

	if !addressGroup.Meta.CreationTS.IsZero() {
		protoAG.Meta.CreationTs = timestamppb.New(addressGroup.Meta.CreationTS.Time)
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
		ServiceRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       protoBinding.ServiceRef.Identifier.Name,
			},
			Namespace: protoBinding.ServiceRef.Identifier.Namespace,
		},
		AddressGroupRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       protoBinding.AddressGroupRef.Identifier.Name,
			},
			Namespace: protoBinding.AddressGroupRef.Identifier.Namespace,
		},
	}

	// meta
	if protoBinding.Meta != nil {
		binding.Meta = models.Meta{
			UID:                protoBinding.Meta.Uid,
			ResourceVersion:    protoBinding.Meta.ResourceVersion,
			Generation:         protoBinding.Meta.Generation,
			Labels:             protoBinding.Meta.Labels,
			Annotations:        protoBinding.Meta.Annotations,
			GeneratedName:      protoBinding.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(protoBinding.Meta.Conditions),
			ObservedGeneration: protoBinding.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(protoBinding.Meta.ManagedFields),
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
				Name:      binding.ServiceRef.Name,
				Namespace: binding.Namespace, // Service is in same namespace as binding
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: binding.ServiceRef.APIVersion,
				Kind:       binding.ServiceRef.Kind,
				Name:       binding.ServiceRef.Name,
				Namespace:  binding.Namespace, // Service is in same namespace as binding
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      binding.AddressGroupRef.Name,
				Namespace: binding.AddressGroupRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: binding.AddressGroupRef.APIVersion,
				Kind:       binding.AddressGroupRef.Kind,
				Name:       binding.AddressGroupRef.Name,
				Namespace:  binding.AddressGroupRef.Namespace,
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
			GeneratedName:   binding.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(binding.Meta.ManagedFields),
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
		serviceRef := models.NewServiceRef(
			protoSPR.Identifier.Name,
			models.WithNamespace(protoSPR.Identifier.Namespace),
		)

		servicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		// Конвертация ProtocolPorts
		for protocol, protoRanges := range protoSPR.Ports.Ports {
			var ranges []models.PortRange
			for _, protoRange := range protoRanges.Ranges {
				ranges = append(ranges, models.PortRange{
					Start: int(protoRange.Start),
					End:   int(protoRange.End),
				})
			}
			servicePorts.Ports[models.TransportProtocol(protocol)] = ranges
		}

		mapping.AccessPorts[serviceRef] = servicePorts
	}

	// meta
	if proto.Meta != nil {
		mapping.Meta = models.Meta{
			UID:                proto.Meta.Uid,
			ResourceVersion:    proto.Meta.ResourceVersion,
			Generation:         proto.Meta.Generation,
			Labels:             proto.Meta.Labels,
			Annotations:        proto.Meta.Annotations,
			GeneratedName:      proto.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(proto.Meta.Conditions),
			ObservedGeneration: proto.Meta.ObservedGeneration,
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
				Name:      serviceRef.Name,
				Namespace: serviceRef.Namespace,
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
			GeneratedName:   m.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(m.Meta.ManagedFields),
		}
		proto.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	return proto
}

// RuleS2S конверторы
func convertRuleS2SFromProto(proto *netguardpb.RuleS2S) models.RuleS2S {
	// Конвертация Traffic protobuf enum в string
	var traffic models.Traffic
	switch proto.Traffic {
	case netguardpb.Traffic_Ingress:
		traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		traffic = models.EGRESS
	default:
		traffic = models.INGRESS // default
	}

	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		Traffic: traffic,
		ServiceLocalRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "ServiceAlias",
				Name:       proto.ServiceLocalRef.Identifier.Name,
			},
			Namespace: proto.ServiceLocalRef.Identifier.Namespace,
		},
		ServiceRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "ServiceAlias",
				Name:       proto.ServiceRef.Identifier.Name,
			},
			Namespace: proto.ServiceRef.Identifier.Namespace,
		},
		Trace: proto.Trace, // Copy trace field from proto
	}

	// Convert IEAgAgRuleRefs
	if len(proto.IeagAgRuleRefs) > 0 {
		rule.IEAgAgRuleRefs = make([]v1beta1.NamespacedObjectReference, len(proto.IeagAgRuleRefs))
		for i, ref := range proto.IeagAgRuleRefs {
			rule.IEAgAgRuleRefs[i] = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "IEAgAgRule",
					Name:       ref.Name,
				},
				Namespace: ref.Namespace,
			}
		}
	}

	// meta
	if proto.Meta != nil {
		rule.Meta = models.Meta{
			UID:                proto.Meta.Uid,
			ResourceVersion:    proto.Meta.ResourceVersion,
			Generation:         proto.Meta.Generation,
			Labels:             proto.Meta.Labels,
			Annotations:        proto.Meta.Annotations,
			GeneratedName:      proto.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(proto.Meta.Conditions),
			ObservedGeneration: proto.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(proto.Meta.ManagedFields),
		}
		if proto.Meta.CreationTs != nil {
			rule.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return rule
}

func convertRuleS2SToProto(m models.RuleS2S) *netguardpb.RuleS2S {
	// Конвертация Traffic string в protobuf enum
	var traffic netguardpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = netguardpb.Traffic_Ingress
	case models.EGRESS:
		traffic = netguardpb.Traffic_Egress
	default:
		traffic = netguardpb.Traffic_Ingress // default
	}

	proto := &netguardpb.RuleS2S{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		Traffic: traffic,
		ServiceLocalRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceLocalRef.Name,
				Namespace: m.ServiceLocalRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.ServiceLocalRef.APIVersion,
				Kind:       m.ServiceLocalRef.Kind,
				Name:       m.ServiceLocalRef.Name,
				Namespace:  m.ServiceLocalRef.Namespace,
			},
		},
		ServiceRef: &netguardpb.ServiceRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.ServiceRef.Name,
				Namespace: m.ServiceRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.ServiceRef.APIVersion,
				Kind:       m.ServiceRef.Kind,
				Name:       m.ServiceRef.Name,
				Namespace:  m.ServiceRef.Namespace,
			},
		},
		Trace: m.Trace, // Copy trace field to proto
	}

	// Convert IEAgAgRuleRefs
	if len(m.IEAgAgRuleRefs) > 0 {
		proto.IeagAgRuleRefs = make([]*netguardpb.ResourceIdentifier, len(m.IEAgAgRuleRefs))
		for i, ref := range m.IEAgAgRuleRefs {
			proto.IeagAgRuleRefs[i] = &netguardpb.ResourceIdentifier{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			}
		}
	}

	if !m.Meta.CreationTS.IsZero() {
		proto.Meta = &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
			GeneratedName:   m.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(m.Meta.ManagedFields),
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
		ServiceRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       proto.ServiceRef.Identifier.Name,
			},
			Namespace: proto.ServiceRef.Identifier.Namespace,
		},
		Meta: models.Meta{},
	}

	if proto.Meta != nil {
		alias.Meta = models.Meta{
			UID:                proto.Meta.Uid,
			ResourceVersion:    proto.Meta.ResourceVersion,
			Generation:         proto.Meta.Generation,
			Labels:             proto.Meta.Labels,
			Annotations:        proto.Meta.Annotations,
			GeneratedName:      proto.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(proto.Meta.Conditions),
			ObservedGeneration: proto.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(proto.Meta.ManagedFields),
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
				Name:      m.ServiceRef.Name,
				Namespace: m.Namespace, // Service is in same namespace as alias
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.ServiceRef.APIVersion,
				Kind:       m.ServiceRef.Kind,
				Name:       m.ServiceRef.Name,
				Namespace:  m.Namespace, // Service is in same namespace as alias
			},
		},
		Meta: &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
			GeneratedName:   m.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(m.Meta.ManagedFields),
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
		ServiceRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       proto.ServiceRef.Identifier.Name,
			},
			Namespace: proto.ServiceRef.Identifier.Namespace,
		},
		AddressGroupRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       proto.AddressGroupRef.Identifier.Name,
			},
			Namespace: proto.AddressGroupRef.Identifier.Namespace,
		},
		Meta: models.Meta{},
	}
	if proto.Meta != nil {
		policy.Meta = models.Meta{
			UID:                proto.Meta.Uid,
			ResourceVersion:    proto.Meta.ResourceVersion,
			Generation:         proto.Meta.Generation,
			Labels:             proto.Meta.Labels,
			Annotations:        proto.Meta.Annotations,
			GeneratedName:      proto.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(proto.Meta.Conditions),
			ObservedGeneration: proto.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(proto.Meta.ManagedFields),
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
				Name:      m.ServiceRef.Name,
				Namespace: m.ServiceRef.Namespace,
			},
			// ServiceRef in protobuf now uses NamespacedObjectReference
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.ServiceRef.APIVersion,
				Kind:       m.ServiceRef.Kind,
				Name:       m.ServiceRef.Name,
				Namespace:  m.ServiceRef.Namespace,
			},
		},
		AddressGroupRef: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.AddressGroupRef.Name,
				Namespace: m.AddressGroupRef.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.AddressGroupRef.APIVersion,
				Kind:       m.AddressGroupRef.Kind,
				Name:       m.AddressGroupRef.Name,
				Namespace:  m.AddressGroupRef.Namespace,
			},
		},
		Meta: &netguardpb.Meta{
			Uid:             m.Meta.UID,
			ResourceVersion: m.Meta.ResourceVersion,
			Generation:      m.Meta.Generation,
			Labels:          m.Meta.Labels,
			Annotations:     m.Meta.Annotations,
			GeneratedName:   m.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(m.Meta.ManagedFields),
		},
	}
	if !m.Meta.CreationTS.IsZero() {
		protoPol.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}
	return protoPol
}

// IEAgAgRule конверторы
// ConvertIEAgAgRuleFromProto converts protobuf IEAgAgRule to domain model
func ConvertIEAgAgRuleFromProto(proto *netguardpb.IEAgAgRule) models.IEAgAgRule {
	// Конвертация Transport protobuf enum в string
	var transport models.TransportProtocol
	switch proto.Transport {
	case netguardpb.Networks_NetIP_TCP:
		transport = models.TCP
	case netguardpb.Networks_NetIP_UDP:
		transport = models.UDP
	default:
		transport = models.TCP // default
	}

	// Конвертация Traffic protobuf enum в string
	var traffic models.Traffic
	switch proto.Traffic {
	case netguardpb.Traffic_Ingress:
		traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		traffic = models.EGRESS
	default:
		traffic = models.INGRESS // default
	}

	// Конвертация Action protobuf enum в правильное значение
	var action models.RuleAction
	switch proto.Action {
	case netguardpb.RuleAction_ACCEPT:
		action = models.ActionAccept
	case netguardpb.RuleAction_DROP:
		action = models.ActionDrop
	default:
		action = models.ActionAccept // default
	}

	rule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		Transport: transport,
		Traffic:   traffic,
		AddressGroupLocal: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       proto.AddressGroupLocal.Identifier.Name,
			},
			Namespace: proto.AddressGroupLocal.Identifier.Namespace,
		},
		AddressGroup: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       proto.AddressGroup.Identifier.Name,
			},
			Namespace: proto.AddressGroup.Identifier.Namespace,
		},
		Action:   action,
		Logs:     proto.Logs,
		Trace:    proto.Trace, // Copy trace field from proto
		Priority: proto.Priority,
	}

	// Конвертация Ports
	if len(proto.Ports) > 0 {
		rule.Ports = make([]models.PortSpec, len(proto.Ports))
		for i, portSpec := range proto.Ports {
			rule.Ports[i] = models.PortSpec{
				Source:      portSpec.Source,
				Destination: portSpec.Destination,
			}
		}
	}

	// meta
	if proto.Meta != nil {
		rule.Meta = models.Meta{
			UID:                proto.Meta.Uid,
			ResourceVersion:    proto.Meta.ResourceVersion,
			Generation:         proto.Meta.Generation,
			Labels:             proto.Meta.Labels,
			Annotations:        proto.Meta.Annotations,
			GeneratedName:      proto.Meta.GeneratedName,
			Conditions:         models.ProtoConditionsToK8s(proto.Meta.Conditions),
			ObservedGeneration: proto.Meta.ObservedGeneration,
			ManagedFields:      convertManagedFieldsFromProto(proto.Meta.ManagedFields),
		}
		if proto.Meta.CreationTs != nil {
			rule.Meta.CreationTS = metav1.NewTime(proto.Meta.CreationTs.AsTime())
		}
	}

	return rule
}

func convertIEAgAgRuleToProto(m models.IEAgAgRule) *netguardpb.IEAgAgRule {
	// Конвертация Transport string в protobuf enum
	var transport netguardpb.Networks_NetIP_Transport
	switch m.Transport {
	case models.TCP:
		transport = netguardpb.Networks_NetIP_TCP
	case models.UDP:
		transport = netguardpb.Networks_NetIP_UDP
	default:
		transport = netguardpb.Networks_NetIP_TCP // default
	}

	// Конвертация Traffic string в protobuf enum
	var traffic netguardpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = netguardpb.Traffic_Ingress
	case models.EGRESS:
		traffic = netguardpb.Traffic_Egress
	default:
		traffic = netguardpb.Traffic_Ingress // default
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
				Name:      m.AddressGroupLocal.Name,
				Namespace: m.AddressGroupLocal.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.AddressGroupLocal.APIVersion,
				Kind:       m.AddressGroupLocal.Kind,
				Name:       m.AddressGroupLocal.Name,
				Namespace:  m.AddressGroupLocal.Namespace,
			},
		},
		AddressGroup: &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      m.AddressGroup.Name,
				Namespace: m.AddressGroup.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.AddressGroup.APIVersion,
				Kind:       m.AddressGroup.Kind,
				Name:       m.AddressGroup.Name,
				Namespace:  m.AddressGroup.Namespace,
			},
		},
		Action:   netguardpb.RuleAction(netguardpb.RuleAction_value[string(m.Action)]),
		Logs:     m.Logs,
		Trace:    m.Trace, // Copy trace field to proto
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
			GeneratedName:   m.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(m.Meta.ManagedFields),
		}
		proto.Meta.CreationTs = timestamppb.New(m.Meta.CreationTS.Time)
	}

	return proto
}
