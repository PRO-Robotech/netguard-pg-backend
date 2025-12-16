package converters

import (
	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertSvcFqdnRule converts protobuf SvcFqdnRule to domain model
func ConvertSvcFqdnRule(rule *netguardpb.SvcFqdnRule) models.SvcFqdnRule {
	if rule == nil {
		return models.SvcFqdnRule{}
	}

	result := models.SvcFqdnRule{
		SelfRef:     models.NewSelfRef(GetSelfRef(rule.GetSelfRef())),
		FQDN:        rule.GetFqdn(),
		Logs:        rule.GetLogs(),
		Trace:       rule.GetTrace(),
		Action:      ConvertActionFromPB(rule.GetAction()),
		Priority:    rule.GetPriority(),
		Description: rule.GetDescription(),
		Comment:     rule.GetComment(),
		Meta:        ConvertMeta(rule.Meta),
	}

	// Convert ServiceFrom reference (NamespacedObjectReference)
	if svcFrom := rule.GetServiceFrom(); svcFrom != nil {
		result.ServiceFromRef = NewNamespacedObjectReference(
			KindService,
			svcFrom.GetName(),
			svcFrom.GetNamespace(),
		)
	}

	// Convert transport
	result.Transport = ConvertTransportFromPB(rule.GetTransport())

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]models.FqdnPortSpec, len(rule.Ports))
		for i, p := range rule.Ports {
			result.Ports[i] = models.FqdnPortSpec{
				Port: p.GetPort(),
			}
		}
	}

	return result
}

// ConvertSvcFqdnRuleToPB converts domain SvcFqdnRule to protobuf
func ConvertSvcFqdnRuleToPB(rule models.SvcFqdnRule) *netguardpb.SvcFqdnRule {
	pb := &netguardpb.SvcFqdnRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		ServiceFrom: &netguardpb.NamespacedObjectReference{
			ApiVersion: rule.ServiceFromRef.APIVersion,
			Kind:       rule.ServiceFromRef.Kind,
			Name:       rule.ServiceFromRef.Name,
			Namespace:  rule.ServiceFromRef.Namespace,
		},
		Fqdn:        rule.FQDN,
		Transport:   ConvertTransportToPB(rule.Transport),
		Logs:        rule.Logs,
		Trace:       rule.Trace,
		Action:      ConvertActionToPB(rule.Action),
		Priority:    rule.Priority,
		Description: rule.Description,
		Comment:     rule.Comment,
		Meta:        ConvertMetaToPB(rule.Meta),
	}

	if len(rule.Ports) > 0 {
		pb.Ports = make([]*netguardpb.FqdnPortSpec, len(rule.Ports))
		for i, p := range rule.Ports {
			pb.Ports[i] = &netguardpb.FqdnPortSpec{Port: p.Port}
		}
	}

	return pb
}
