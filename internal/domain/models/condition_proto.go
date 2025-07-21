package models

import (
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProtoConditionToK8s converts protobuf Condition to k8s metav1.Condition
func ProtoConditionToK8s(protoCond *netguardpb.Condition) metav1.Condition {
	if protoCond == nil {
		return metav1.Condition{}
	}

	var lastTransitionTime metav1.Time
	if protoCond.LastTransitionTime != nil {
		lastTransitionTime = metav1.Time{Time: protoCond.LastTransitionTime.AsTime()}
	}

	return metav1.Condition{
		Type:               protoCond.Type,
		Status:             metav1.ConditionStatus(protoCond.Status),
		ObservedGeneration: protoCond.ObservedGeneration,
		LastTransitionTime: lastTransitionTime,
		Reason:             protoCond.Reason,
		Message:            protoCond.Message,
	}
}

// K8sConditionToProto converts k8s metav1.Condition to protobuf Condition
func K8sConditionToProto(k8sCond metav1.Condition) *netguardpb.Condition {
	var lastTransitionTime *timestamppb.Timestamp
	if !k8sCond.LastTransitionTime.IsZero() {
		lastTransitionTime = timestamppb.New(k8sCond.LastTransitionTime.Time)
	}

	return &netguardpb.Condition{
		Type:               k8sCond.Type,
		Status:             string(k8sCond.Status),
		ObservedGeneration: k8sCond.ObservedGeneration,
		LastTransitionTime: lastTransitionTime,
		Reason:             k8sCond.Reason,
		Message:            k8sCond.Message,
	}
}

// ProtoConditionsToK8s converts protobuf Conditions to k8s metav1.Conditions
func ProtoConditionsToK8s(protoConditions []*netguardpb.Condition) []metav1.Condition {
	if len(protoConditions) == 0 {
		return nil
	}

	k8sConditions := make([]metav1.Condition, 0, len(protoConditions))
	for _, protoCond := range protoConditions {
		if protoCond != nil {
			k8sConditions = append(k8sConditions, ProtoConditionToK8s(protoCond))
		}
	}

	return k8sConditions
}

// K8sConditionsToProto converts k8s metav1.Conditions to protobuf Conditions
func K8sConditionsToProto(k8sConditions []metav1.Condition) []*netguardpb.Condition {
	if len(k8sConditions) == 0 {
		return nil
	}

	protoConditions := make([]*netguardpb.Condition, 0, len(k8sConditions))
	for _, k8sCond := range k8sConditions {
		protoConditions = append(protoConditions, K8sConditionToProto(k8sCond))
	}

	return protoConditions
}

// ProtoMetaToK8sConditions extracts conditions from protobuf Meta to k8s format
func ProtoMetaToK8sConditions(protoMeta *netguardpb.Meta) []metav1.Condition {
	if protoMeta == nil {
		return nil
	}

	return ProtoConditionsToK8s(protoMeta.Conditions)
}

// K8sConditionsToProtoMeta sets conditions in protobuf Meta from k8s format
func K8sConditionsToProtoMeta(k8sConditions []metav1.Condition, protoMeta *netguardpb.Meta) {
	if protoMeta == nil {
		return
	}

	protoMeta.Conditions = K8sConditionsToProto(k8sConditions)
}
