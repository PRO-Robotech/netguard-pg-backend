package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConvertRuleS2S converts protobuf RuleS2S to domain model
func ConvertRuleS2S(r *netguardpb.RuleS2S) models.RuleS2S {
	result := models.RuleS2S{
		SelfRef: models.NewSelfRef(GetSelfRef(r.GetSelfRef())),
		Meta:    ConvertMeta(r.Meta),
	}

	result.Traffic = ConvertTrafficFromPB(r.Traffic)
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
	result.ServiceLocalRef = NewNamespacedObjectReference(KindService, localName, localNamespace)

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
	result.ServiceRef = NewNamespacedObjectReference(KindService, serviceName, serviceNamespace)

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
			result.IEAgAgRuleRefs[i] = NewNamespacedObjectReference(KindIEAgAgRule, ref.Name, ref.Namespace)
		}
	}

	return result
}

// ConvertRuleS2SToPB converts domain model to protobuf RuleS2S
func ConvertRuleS2SToPB(r models.RuleS2S) *netguardpb.RuleS2S {
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

	pb.Traffic = ConvertTrafficToPB(r.Traffic)
	pb.Meta = &netguardpb.Meta{
		Uid:                r.Meta.UID,
		ResourceVersion:    r.Meta.ResourceVersion,
		Generation:         r.Meta.Generation,
		Labels:             r.Meta.Labels,
		Annotations:        r.Meta.Annotations,
		Conditions:         models.K8sConditionsToProto(r.Meta.Conditions),
		ObservedGeneration: r.Meta.ObservedGeneration,
	}
	if !r.Meta.CreationTS.IsZero() {
		pb.Meta.CreationTs = timestamppb.New(r.Meta.CreationTS.Time)
	}

	return pb
}

// ConvertIEAgAgRuleToPB converts domain model to protobuf IEAgAgRule
func ConvertIEAgAgRuleToPB(rule models.IEAgAgRule) *netguardpb.IEAgAgRule {

	result := &netguardpb.IEAgAgRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Transport: ConvertTransportToPB(rule.Transport),
		Traffic:   ConvertTrafficToPB(rule.Traffic),
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
		Action:   ConvertActionToPB(rule.Action),
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
		Conditions:         models.K8sConditionsToProto(rule.Meta.Conditions),
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
