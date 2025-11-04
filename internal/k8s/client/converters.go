package client

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

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
		if protoField.Time != nil {
			managedFields[i].Time = &metav1.Time{
				Time: protoField.Time.AsTime(),
			}
		}
		if len(protoField.FieldsV1) > 0 {
			managedFields[i].FieldsV1 = &metav1.FieldsV1{
				Raw: protoField.FieldsV1,
			}
		}
	}
	return managedFields
}
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
		if field.Time != nil {
			protoFields[i].Time = timestamppb.New(field.Time.Time)
		}
		if field.FieldsV1 != nil {
			protoFields[i].FieldsV1 = field.FieldsV1.Raw
		}
	}
	return protoFields
}
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
		if protoSvc.Meta.DeletionTs != nil {
			service.Meta.DeletionTS = &metav1.Time{Time: protoSvc.Meta.DeletionTs.AsTime()}
		}
	}
	for _, port := range protoSvc.IngressPorts {
		var protocol models.TransportProtocol
		switch port.Protocol {
		case netguardpb.Networks_NetIP_TCP:
			protocol = models.TCP
		case netguardpb.Networks_NetIP_UDP:
			protocol = models.UDP
		default:
			protocol = models.TCP
		}
		service.IngressPorts = append(service.IngressPorts, models.IngressPort{
			Protocol:    protocol,
			Port:        port.Port,
			Description: port.Description,
		})
	}
	for _, agRef := range protoSvc.AddressGroups {
		service.AddressGroups = append(service.AddressGroups, models.NewAddressGroupRef(
			agRef.Identifier.Name,
			models.WithNamespace(agRef.Identifier.Namespace),
		))
	}
	if len(protoSvc.AggregatedAddressGroups) > 0 {
		service.AggregatedAddressGroups = make([]models.AddressGroupReference, len(protoSvc.AggregatedAddressGroups))
		for i, agRef := range protoSvc.AggregatedAddressGroups {
			service.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref: v1beta1.NamespacedObjectReference{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: agRef.Ref.ApiVersion,
						Kind:       agRef.Ref.Kind,
						Name:       agRef.Ref.Name,
					},
					Namespace: agRef.Ref.Namespace,
				},
				Source: convertAGSourceFromPB(agRef.Source),
			}
		}
	}
	if protoSvc.XSvcsvcRules != nil {
		service.XSvcSvcRules = &models.XSvcSvcRules{}
		if len(protoSvc.XSvcsvcRules.AsServiceFrom) > 0 {
			service.XSvcSvcRules.AsServiceFrom = make([]v1beta1.NamespacedObjectReference, len(protoSvc.XSvcsvcRules.AsServiceFrom))
			for i, ref := range protoSvc.XSvcsvcRules.AsServiceFrom {
				service.XSvcSvcRules.AsServiceFrom[i] = v1beta1.NamespacedObjectReference{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "SvcSvcRule",
						Name:       ref.Name,
					},
					Namespace: ref.Namespace,
				}
			}
		}
		if len(protoSvc.XSvcsvcRules.AsServiceTo) > 0 {
			service.XSvcSvcRules.AsServiceTo = make([]v1beta1.NamespacedObjectReference, len(protoSvc.XSvcsvcRules.AsServiceTo))
			for i, ref := range protoSvc.XSvcsvcRules.AsServiceTo {
				service.XSvcSvcRules.AsServiceTo[i] = v1beta1.NamespacedObjectReference{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "SvcSvcRule",
						Name:       ref.Name,
					},
					Namespace: ref.Namespace,
				}
			}
		}
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
	for _, port := range service.IngressPorts {
		var protocol netguardpb.Networks_NetIP_Transport
		switch port.Protocol {
		case models.TCP:
			protocol = netguardpb.Networks_NetIP_TCP
		case models.UDP:
			protocol = netguardpb.Networks_NetIP_UDP
		default:
			protocol = netguardpb.Networks_NetIP_TCP
		}
		protoSvc.IngressPorts = append(protoSvc.IngressPorts, &netguardpb.IngressPort{
			Protocol:    protocol,
			Port:        port.Port,
			Description: port.Description,
		})
	}
	for _, agRef := range service.AddressGroups {
		protoSvc.AddressGroups = append(protoSvc.AddressGroups, &netguardpb.AddressGroupRef{
			Identifier: &netguardpb.ResourceIdentifier{
				Name:      agRef.Name,
				Namespace: agRef.Namespace,
			},
		})
	}
	if len(service.AggregatedAddressGroups) > 0 {
		protoSvc.AggregatedAddressGroups = make([]*netguardpb.AddressGroupReference, len(service.AggregatedAddressGroups))
		for i, agRef := range service.AggregatedAddressGroups {
			protoSvc.AggregatedAddressGroups[i] = &netguardpb.AddressGroupReference{
				Ref: &netguardpb.NamespacedObjectReference{
					ApiVersion: agRef.Ref.APIVersion,
					Kind:       agRef.Ref.Kind,
					Name:       agRef.Ref.Name,
					Namespace:  agRef.Ref.Namespace,
				},
				Source: convertAGSourceToPB(agRef.Source),
			}
		}
	}
	return protoSvc
}
func convertAGSourceFromPB(source netguardpb.AddressGroupRegistrationSource) models.AddressGroupRegistrationSource {
	switch source {
	case netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC:
		return models.AddressGroupSourceSpec
	case netguardpb.AddressGroupRegistrationSource_AG_SOURCE_BINDING:
		return models.AddressGroupSourceBinding
	default:
		return models.AddressGroupSourceSpec
	}
}
func convertAGSourceToPB(source models.AddressGroupRegistrationSource) netguardpb.AddressGroupRegistrationSource {
	switch source {
	case models.AddressGroupSourceSpec:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC
	case models.AddressGroupSourceBinding:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_BINDING
	default:
		return netguardpb.AddressGroupRegistrationSource_AG_SOURCE_SPEC
	}
}
func convertAddressGroupFromProto(protoAG *netguardpb.AddressGroup) models.AddressGroup {
	var defaultAction models.RuleAction
	switch protoAG.DefaultAction {
	case netguardpb.RuleAction_ACCEPT:
		defaultAction = models.ActionAccept
	case netguardpb.RuleAction_DROP:
		defaultAction = models.ActionDrop
	default:
		defaultAction = models.ActionDrop
	}
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				protoAG.SelfRef.Name,
				models.WithNamespace(protoAG.SelfRef.Namespace),
			),
		},
		DefaultAction:    defaultAction,
		Logs:             protoAG.Logs,
		Trace:            protoAG.Trace,
		AddressGroupName: protoAG.AddressGroupName,
	}
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
		if protoAG.Meta.DeletionTs != nil {
			addressGroup.Meta.DeletionTS = &metav1.Time{Time: protoAG.Meta.DeletionTs.AsTime()}
		}
	}
	for _, protoNetworkItem := range protoAG.Networks {
		addressGroup.Networks = append(addressGroup.Networks, models.NetworkItem{
			Name:       protoNetworkItem.Name,
			CIDR:       protoNetworkItem.Cidr,
			ApiVersion: protoNetworkItem.ApiVersion,
			Kind:       protoNetworkItem.Kind,
			Namespace:  protoNetworkItem.Namespace,
		})
	}
	if len(protoAG.Hosts) > 0 {
		addressGroup.Hosts = make([]v1beta1.NamespacedObjectReference, len(protoAG.Hosts))
		for i, host := range protoAG.Hosts {
			addressGroup.Hosts[i] = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: host.ApiVersion,
					Kind:       host.Kind,
					Name:       host.Name,
				},
				Namespace: host.Namespace,
			}
		}
	}
	if len(protoAG.AggregatedHosts) > 0 {
		addressGroup.AggregatedHosts = make([]models.HostReference, len(protoAG.AggregatedHosts))
		for i, hostRef := range protoAG.AggregatedHosts {
			addressGroup.AggregatedHosts[i] = models.HostReference{
				Ref: v1beta1.NamespacedObjectReference{
					ObjectReference: v1beta1.ObjectReference{
						APIVersion: hostRef.Ref.ApiVersion,
						Kind:       hostRef.Ref.Kind,
						Name:       hostRef.Ref.Name,
					},
					Namespace: hostRef.Ref.Namespace,
				},
				UUID:   hostRef.Uuid,
				Source: convertHostRegistrationSourceFromPB(hostRef.Source),
			}
		}
	}
	return addressGroup
}
func convertHostRegistrationSourceFromPB(source netguardpb.HostRegistrationSource) models.HostRegistrationSource {
	switch source {
	case netguardpb.HostRegistrationSource_HOST_SOURCE_SPEC:
		return models.HostSourceSpec
	case netguardpb.HostRegistrationSource_HOST_SOURCE_BINDING:
		return models.HostSourceBinding
	default:
		return models.HostSourceSpec
	}
}
func convertAddressGroupToProto(addressGroup models.AddressGroup) *netguardpb.AddressGroup {
	var defaultAction netguardpb.RuleAction
	switch addressGroup.DefaultAction {
	case models.ActionAccept:
		defaultAction = netguardpb.RuleAction_ACCEPT
	case models.ActionDrop:
		defaultAction = netguardpb.RuleAction_DROP
	default:
		defaultAction = netguardpb.RuleAction_DROP
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
	if len(addressGroup.Networks) > 0 {
		protoAG.Networks = make([]*netguardpb.NetworkItem, len(addressGroup.Networks))
		for i, network := range addressGroup.Networks {
			protoAG.Networks[i] = &netguardpb.NetworkItem{
				Name:       network.Name,
				Cidr:       network.CIDR,
				ApiVersion: network.ApiVersion,
				Kind:       network.Kind,
				Namespace:  network.Namespace,
			}
		}
	}
	if len(addressGroup.Hosts) > 0 {
		protoAG.Hosts = make([]*netguardpb.NamespacedObjectReference, len(addressGroup.Hosts))
		for i, host := range addressGroup.Hosts {
			protoAG.Hosts[i] = &netguardpb.NamespacedObjectReference{
				ApiVersion: host.APIVersion,
				Kind:       host.Kind,
				Name:       host.Name,
				Namespace:  host.Namespace,
			}
		}
	}
	return protoAG
}
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
		if protoBinding.Meta.DeletionTs != nil {
			binding.Meta.DeletionTS = &metav1.Time{Time: protoBinding.Meta.DeletionTs.AsTime()}
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
				Namespace: binding.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: binding.ServiceRef.APIVersion,
				Kind:       binding.ServiceRef.Kind,
				Name:       binding.ServiceRef.Name,
				Namespace:  binding.Namespace,
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
	for _, protoSPR := range proto.AccessPorts {
		serviceRef := models.NewServiceRef(
			protoSPR.Identifier.Name,
			models.WithNamespace(protoSPR.Identifier.Namespace),
		)
		servicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}
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
		if proto.Meta.DeletionTs != nil {
			mapping.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
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
func convertRuleS2SFromProto(proto *netguardpb.RuleS2S) models.RuleS2S {
	var traffic models.Traffic
	switch proto.Traffic {
	case netguardpb.Traffic_Ingress:
		traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		traffic = models.EGRESS
	default:
		traffic = models.INGRESS
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
				Kind:       "Service",
				Name:       proto.ServiceLocalRef.Identifier.Name,
			},
			Namespace: proto.ServiceLocalRef.Identifier.Namespace,
		},
		ServiceRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       proto.ServiceRef.Identifier.Name,
			},
			Namespace: proto.ServiceRef.Identifier.Namespace,
		},
		Trace: proto.Trace,
	}
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
		if proto.Meta.DeletionTs != nil {
			rule.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
		}
	}
	return rule
}
func convertRuleS2SToProto(m models.RuleS2S) *netguardpb.RuleS2S {
	var traffic netguardpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = netguardpb.Traffic_Ingress
	case models.EGRESS:
		traffic = netguardpb.Traffic_Egress
	default:
		traffic = netguardpb.Traffic_Ingress
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
		Trace: m.Trace,
	}
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
		if proto.Meta.DeletionTs != nil {
			alias.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
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
				Namespace: m.Namespace,
			},
			ObjectRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: m.ServiceRef.APIVersion,
				Kind:       m.ServiceRef.Kind,
				Name:       m.ServiceRef.Name,
				Namespace:  m.Namespace,
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
		if proto.Meta.DeletionTs != nil {
			policy.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
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
func ConvertIEAgAgRuleFromProto(proto *netguardpb.IEAgAgRule) models.IEAgAgRule {
	var transport models.TransportProtocol
	switch proto.Transport {
	case netguardpb.Networks_NetIP_TCP:
		transport = models.TCP
	case netguardpb.Networks_NetIP_UDP:
		transport = models.UDP
	default:
		transport = models.TCP
	}
	var traffic models.Traffic
	switch proto.Traffic {
	case netguardpb.Traffic_Ingress:
		traffic = models.INGRESS
	case netguardpb.Traffic_Egress:
		traffic = models.EGRESS
	default:
		traffic = models.INGRESS
	}
	var action models.RuleAction
	switch proto.Action {
	case netguardpb.RuleAction_ACCEPT:
		action = models.ActionAccept
	case netguardpb.RuleAction_DROP:
		action = models.ActionDrop
	default:
		action = models.ActionAccept
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
		Trace:    proto.Trace,
		Priority: proto.Priority,
	}
	if len(proto.Ports) > 0 {
		rule.Ports = make([]models.PortSpec, len(proto.Ports))
		for i, portSpec := range proto.Ports {
			rule.Ports[i] = models.PortSpec{
				Source:      portSpec.Source,
				Destination: portSpec.Destination,
			}
		}
	}
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
		if proto.Meta.DeletionTs != nil {
			rule.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
		}
	}
	return rule
}
func convertIEAgAgRuleToProto(m models.IEAgAgRule) *netguardpb.IEAgAgRule {
	var transport netguardpb.Networks_NetIP_Transport
	switch m.Transport {
	case models.TCP:
		transport = netguardpb.Networks_NetIP_TCP
	case models.UDP:
		transport = netguardpb.Networks_NetIP_UDP
	default:
		transport = netguardpb.Networks_NetIP_TCP
	}
	var traffic netguardpb.Traffic
	switch m.Traffic {
	case models.INGRESS:
		traffic = netguardpb.Traffic_Ingress
	case models.EGRESS:
		traffic = netguardpb.Traffic_Egress
	default:
		traffic = netguardpb.Traffic_Ingress
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
		Trace:    m.Trace,
		Priority: m.Priority,
	}
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
func convertSvcSvcRuleFromProto(proto *netguardpb.SvcSvcRule) models.SvcSvcRule {
	rule := models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       proto.ServiceFrom.Name,
			},
			Namespace: proto.ServiceFrom.Namespace,
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       proto.ServiceTo.Name,
			},
			Namespace: proto.ServiceTo.Namespace,
		},
		Action:   models.RuleAction(proto.Action.String()),
		Priority: proto.Priority,
		Logs:     proto.Logs,
		Trace:    proto.Trace,
	}
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
		if proto.Meta.DeletionTs != nil {
			rule.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
		}
	}
	return rule
}
func convertSvcSvcRuleToProto(m models.SvcSvcRule) *netguardpb.SvcSvcRule {
	proto := &netguardpb.SvcSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      m.ResourceIdentifier.Name,
			Namespace: m.ResourceIdentifier.Namespace,
		},
		ServiceFrom: &netguardpb.ResourceIdentifier{
			Name:      m.ServiceFromRef.Name,
			Namespace: m.ServiceFromRef.Namespace,
		},
		ServiceTo: &netguardpb.ResourceIdentifier{
			Name:      m.ServiceToRef.Name,
			Namespace: m.ServiceToRef.Namespace,
		},
		Action:   netguardpb.RuleAction(netguardpb.RuleAction_value[string(m.Action)]),
		Priority: m.Priority,
		Logs:     m.Logs,
		Trace:    m.Trace,
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

func convertSvcFqdnRuleFromProto(proto *netguardpb.SvcFqdnRule) models.SvcFqdnRule {
	var transport models.TransportProtocol
	switch proto.Transport {
	case netguardpb.Networks_NetIP_TCP:
		transport = models.TCP
	case netguardpb.Networks_NetIP_UDP:
		transport = models.UDP
	default:
		transport = models.TCP
	}
	rule := models.SvcFqdnRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				proto.SelfRef.Name,
				models.WithNamespace(proto.SelfRef.Namespace),
			),
		},
		FQDN:        proto.Fqdn,
		Transport:   transport,
		Action:      models.RuleAction(proto.Action.String()),
		Priority:    proto.Priority,
		Logs:        proto.Logs,
		Trace:       proto.Trace,
		Description: proto.Description,
	}
	if proto.ServiceFrom != nil {
		rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: proto.ServiceFrom.ApiVersion,
				Kind:       proto.ServiceFrom.Kind,
				Name:       proto.ServiceFrom.Name,
			},
			Namespace: proto.ServiceFrom.Namespace,
		}
	}
	if len(proto.Ports) > 0 {
		rule.Ports = make([]models.FqdnPortSpec, len(proto.Ports))
		for i, port := range proto.Ports {
			rule.Ports[i] = models.FqdnPortSpec{
				Port: port.GetPort(),
			}
		}
	}
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
		if proto.Meta.DeletionTs != nil {
			rule.Meta.DeletionTS = &metav1.Time{Time: proto.Meta.DeletionTs.AsTime()}
		}
	}
	return rule
}

func convertSvcFqdnRuleToProto(rule models.SvcFqdnRule) *netguardpb.SvcFqdnRule {
	var transportProto netguardpb.Networks_NetIP_Transport
	switch rule.Transport {
	case models.UDP:
		transportProto = netguardpb.Networks_NetIP_UDP
	case models.TCP:
		transportProto = netguardpb.Networks_NetIP_TCP
	default:
		transportProto = netguardpb.Networks_NetIP_TCP
	}
	proto := &netguardpb.SvcFqdnRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Fqdn:        rule.FQDN,
		Transport:   transportProto,
		Action:      netguardpb.RuleAction(netguardpb.RuleAction_value[string(rule.Action)]),
		Priority:    rule.Priority,
		Logs:        rule.Logs,
		Trace:       rule.Trace,
		Description: rule.Description,
	}
	if rule.ServiceFromRef.Name != "" {
		proto.ServiceFrom = &netguardpb.NamespacedObjectReference{
			ApiVersion: rule.ServiceFromRef.APIVersion,
			Kind:       rule.ServiceFromRef.Kind,
			Name:       rule.ServiceFromRef.Name,
			Namespace:  rule.ServiceFromRef.Namespace,
		}
	}
	if len(rule.Ports) > 0 {
		proto.Ports = make([]*netguardpb.FqdnPortSpec, len(rule.Ports))
		for i, port := range rule.Ports {
			proto.Ports[i] = &netguardpb.FqdnPortSpec{
				Port: port.Port,
			}
		}
	}
	if !rule.Meta.CreationTS.IsZero() || rule.Meta.UID != "" {
		proto.Meta = &netguardpb.Meta{
			Uid:             rule.Meta.UID,
			ResourceVersion: rule.Meta.ResourceVersion,
			Generation:      rule.Meta.Generation,
			Labels:          rule.Meta.Labels,
			Annotations:     rule.Meta.Annotations,
			GeneratedName:   rule.Meta.GeneratedName,
			ManagedFields:   convertManagedFieldsToProto(rule.Meta.ManagedFields),
		}
		if !rule.Meta.CreationTS.IsZero() {
			proto.Meta.CreationTs = timestamppb.New(rule.Meta.CreationTS.Time)
		}
		if rule.Meta.DeletionTS != nil {
			proto.Meta.DeletionTs = timestamppb.New(rule.Meta.DeletionTS.Time)
		}
	}
	return proto
}
