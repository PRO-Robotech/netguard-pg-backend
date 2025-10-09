package converters

import (
	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetSelfRef extracts ResourceIdentifier from protobuf
func GetSelfRef(identifier *netguardpb.ResourceIdentifier) models.ResourceIdentifier {
	if identifier == nil {
		return models.ResourceIdentifier{}
	}
	return models.NewResourceIdentifier(identifier.GetName(), models.WithNamespace(identifier.GetNamespace()))
}

// ConvertMeta converts protobuf Meta to domain Meta
func ConvertMeta(metaPB *netguardpb.Meta) models.Meta {
	if metaPB == nil {
		return models.Meta{}
	}

	meta := models.Meta{
		UID:                metaPB.Uid,
		ResourceVersion:    metaPB.ResourceVersion,
		Generation:         metaPB.Generation,
		Labels:             metaPB.Labels,
		Annotations:        metaPB.Annotations,
		ObservedGeneration: metaPB.ObservedGeneration,
	}

	if metaPB.CreationTs != nil {
		meta.CreationTS = metav1.NewTime(metaPB.CreationTs.AsTime())
	}

	if metaPB.Conditions != nil {
		meta.Conditions = models.ProtoConditionsToK8s(metaPB.Conditions)
	}

	return meta
}

// ConvertMetaToPB converts domain Meta to protobuf Meta
func ConvertMetaToPB(meta models.Meta) *netguardpb.Meta {
	result := &netguardpb.Meta{
		Uid:                meta.UID,
		ResourceVersion:    meta.ResourceVersion,
		Generation:         meta.Generation,
		Labels:             meta.Labels,
		Annotations:        meta.Annotations,
		Conditions:         models.K8sConditionsToProto(meta.Conditions),
		ObservedGeneration: meta.ObservedGeneration,
	}

	if !meta.CreationTS.IsZero() {
		result.CreationTs = timestamppb.New(meta.CreationTS.Time)
	}

	return result
}

// ResourceIdentifierFromPB converts protobuf ResourceIdentifier to domain model
func ResourceIdentifierFromPB(id *netguardpb.ResourceIdentifier) models.ResourceIdentifier {
	if id == nil {
		return models.ResourceIdentifier{}
	}
	return models.NewResourceIdentifier(id.GetName(), models.WithNamespace(id.GetNamespace()))
}

// ResourceIdentifierToPB converts domain ResourceIdentifier to protobuf
func ResourceIdentifierToPB(id models.ResourceIdentifier) *netguardpb.ResourceIdentifier {
	return &netguardpb.ResourceIdentifier{
		Name:      id.Name,
		Namespace: id.Namespace,
	}
}

// ConvertSyncOp converts protobuf SyncOp to domain SyncOp
func ConvertSyncOp(protoSyncOp netguardpb.SyncOp) models.SyncOp {
	return models.ProtoToSyncOp(int32(protoSyncOp))
}

// ConvertActionToPB converts domain RuleAction to protobuf RuleAction
func ConvertActionToPB(action models.RuleAction) netguardpb.RuleAction {
	switch action {
	case models.ActionAccept:
		return netguardpb.RuleAction_ACCEPT
	case models.ActionDrop:
		return netguardpb.RuleAction_DROP
	default:
		return netguardpb.RuleAction_ACCEPT
	}
}

// ConvertActionFromPB converts protobuf RuleAction to domain RuleAction
func ConvertActionFromPB(action netguardpb.RuleAction) models.RuleAction {
	switch action {
	case netguardpb.RuleAction_ACCEPT:
		return models.ActionAccept
	case netguardpb.RuleAction_DROP:
		return models.ActionDrop
	default:
		return models.ActionAccept
	}
}

// ConvertTransportToPB converts domain TransportProtocol to protobuf Transport
func ConvertTransportToPB(transport models.TransportProtocol) netguardpb.Networks_NetIP_Transport {
	switch transport {
	case models.TCP:
		return netguardpb.Networks_NetIP_TCP
	case models.UDP:
		return netguardpb.Networks_NetIP_UDP
	default:
		return netguardpb.Networks_NetIP_TCP
	}
}

// ConvertTransportFromPB converts protobuf Transport to domain TransportProtocol
func ConvertTransportFromPB(transport netguardpb.Networks_NetIP_Transport) models.TransportProtocol {
	switch transport {
	case netguardpb.Networks_NetIP_TCP:
		return models.TCP
	case netguardpb.Networks_NetIP_UDP:
		return models.UDP
	default:
		return models.TCP
	}
}

// ConvertTrafficToPB converts domain Traffic to protobuf Traffic
func ConvertTrafficToPB(traffic models.Traffic) netguardpb.Traffic {
	switch traffic {
	case models.INGRESS:
		return netguardpb.Traffic_Ingress
	case models.EGRESS:
		return netguardpb.Traffic_Egress
	default:
		return netguardpb.Traffic_Ingress
	}
}

// ConvertTrafficFromPB converts protobuf Traffic to domain Traffic
func ConvertTrafficFromPB(traffic netguardpb.Traffic) models.Traffic {
	switch traffic {
	case netguardpb.Traffic_Ingress:
		return models.INGRESS
	case netguardpb.Traffic_Egress:
		return models.EGRESS
	default:
		return models.INGRESS
	}
}
