package converters

import (
	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertSvcSvcRule converts protobuf SvcSvcRule to domain model
func ConvertSvcSvcRule(r *netguardpb.SvcSvcRule) models.SvcSvcRule {
	if r == nil {
		return models.SvcSvcRule{}
	}

	result := models.SvcSvcRule{
		SelfRef: models.NewSelfRef(GetSelfRef(r.GetSelfRef())),
		Meta:    ConvertMeta(r.Meta),
	}

	// Convert ServiceFrom reference
	if serviceFrom := r.GetServiceFrom(); serviceFrom != nil {
		serviceName := serviceFrom.GetName()
		serviceNamespace := serviceFrom.GetNamespace()
		if serviceName != "" && serviceNamespace != "" {
			result.ServiceFromRef = NewNamespacedObjectReference(
				KindService,
				serviceName,
				serviceNamespace,
			)
		}
	}

	// Convert ServiceTo reference
	if serviceTo := r.GetServiceTo(); serviceTo != nil {
		serviceName := serviceTo.GetName()
		serviceNamespace := serviceTo.GetNamespace()
		if serviceName != "" && serviceNamespace != "" {
			result.ServiceToRef = NewNamespacedObjectReference(
				KindService,
				serviceName,
				serviceNamespace,
			)
		}
	}

	// Convert action enum
	result.Action = ConvertActionFromPB(r.Action)

	// Convert primitive fields
	result.Priority = r.Priority
	result.Logs = r.Logs
	result.Trace = r.Trace
	result.Description = r.Description

	return result
}

// ConvertSvcSvcRuleToPB converts domain SvcSvcRule to protobuf
func ConvertSvcSvcRuleToPB(r models.SvcSvcRule) *netguardpb.SvcSvcRule {
	pb := &netguardpb.SvcSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      r.ResourceIdentifier.Name,
			Namespace: r.ResourceIdentifier.Namespace,
		},
		ServiceFrom: &netguardpb.ResourceIdentifier{
			Name:      r.ServiceFromRef.Name,
			Namespace: r.ServiceFromRef.Namespace,
		},
		ServiceTo: &netguardpb.ResourceIdentifier{
			Name:      r.ServiceToRef.Name,
			Namespace: r.ServiceToRef.Namespace,
		},
		Action:      ConvertActionToPB(r.Action),
		Priority:    r.Priority,
		Logs:        r.Logs,
		Trace:       r.Trace,
		Description: r.Description,
		Meta:        ConvertMetaToPB(r.Meta),
	}

	return pb
}
