package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertIECidrSvcRule converts protobuf IECidrSvcRule to domain model
func ConvertIECidrSvcRule(rule *netguardpb.IECidrSvcRule) models.IECidrSvcRule {
	if rule == nil {
		return models.IECidrSvcRule{}
	}

	result := models.IECidrSvcRule{
		SelfRef:     models.NewSelfRef(GetSelfRef(rule.GetSelfRef())),
		Transport:   ConvertTransportFromPB(rule.GetTransport()),
		CIDR:        rule.GetCidr(),
		Traffic:     ConvertTrafficFromPB(rule.GetTraffic()),
		Logs:        rule.GetLogs(),
		Trace:       rule.GetTrace(),
		Action:      ConvertActionFromPB(rule.GetAction()),
		Priority:    rule.GetPriority(),
		Description: rule.GetDescription(),
		Comment:     rule.GetComment(),
		Meta:        ConvertMeta(rule.Meta),
	}

	// Convert Service reference (NamespacedObjectReference)
	if svc := rule.GetSvc(); svc != nil {
		result.ServiceRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: svc.GetApiVersion(),
				Kind:       svc.GetKind(),
				Name:       svc.GetName(),
			},
			Namespace: svc.GetNamespace(),
		}
	}

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]models.IECidrSvcPortSpec, len(rule.Ports))
		for i, p := range rule.Ports {
			result.Ports[i] = models.IECidrSvcPortSpec{
				S: p.GetS(),
				D: p.GetD(),
			}
		}
	}

	return result
}

// ConvertIECidrSvcRuleToPB converts domain IECidrSvcRule to protobuf
func ConvertIECidrSvcRuleToPB(rule models.IECidrSvcRule) *netguardpb.IECidrSvcRule {
	pb := &netguardpb.IECidrSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Transport: ConvertTransportToPB(rule.Transport),
		Cidr:      rule.CIDR,
		Svc: &netguardpb.NamespacedObjectReference{
			ApiVersion: rule.ServiceRef.APIVersion,
			Kind:       rule.ServiceRef.Kind,
			Name:       rule.ServiceRef.Name,
			Namespace:  rule.ServiceRef.Namespace,
		},
		Traffic:     ConvertTrafficToPB(rule.Traffic),
		Logs:        rule.Logs,
		Trace:       rule.Trace,
		Action:      ConvertActionToPB(rule.Action),
		Priority:    rule.Priority,
		Description: rule.Description,
		Comment:     rule.Comment,
		Meta:        ConvertMetaToPB(rule.Meta),
	}

	if len(rule.Ports) > 0 {
		pb.Ports = make([]*netguardpb.IECidrSvcPortSpec, len(rule.Ports))
		for i, p := range rule.Ports {
			pb.Ports[i] = &netguardpb.IECidrSvcPortSpec{
				S: p.S,
				D: p.D,
			}
		}
	}

	return pb
}
