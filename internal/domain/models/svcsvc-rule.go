package models

import (
	"fmt"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// SvcSvcRule - domain model for service-to-service firewall rule
type SvcSvcRule struct {
	SelfRef                                          // Embedded (Namespace, Name, FQDN)
	ServiceFromRef v1beta1.NamespacedObjectReference // Full K8s object reference
	ServiceToRef   v1beta1.NamespacedObjectReference // Full K8s object reference
	Action         RuleAction                        // ACCEPT or DROP
	Priority       int32                             // 0-1000
	Logs           bool                              // Enable logging
	Trace          bool                              // Enable tracing
	Description    string                            // Optional description
	Comment        string                            // Optional user comment (Netguard-only, not synced to SGROUPS)
	Meta           Meta                              // K8s metadata (resourceVersion, etc.)
}

// ServiceFromRefKey returns "namespace/name" format for map keys and queries
func (r *SvcSvcRule) ServiceFromRefKey() string {
	if r.ServiceFromRef.Namespace == "" {
		return r.ServiceFromRef.Name
	}
	return r.ServiceFromRef.Namespace + "/" + r.ServiceFromRef.Name
}

// ServiceToRefKey returns "namespace/name" format for map keys and queries
func (r *SvcSvcRule) ServiceToRefKey() string {
	if r.ServiceToRef.Namespace == "" {
		return r.ServiceToRef.Name
	}
	return r.ServiceToRef.Namespace + "/" + r.ServiceToRef.Name
}

// GetMeta returns K8s metadata
func (r *SvcSvcRule) GetMeta() Meta {
	return r.Meta
}

// SetMeta sets K8s metadata
func (r *SvcSvcRule) SetMeta(meta Meta) {
	r.Meta = meta
}

// SyncableEntity interface implementation

// GetSyncSubjectType returns the sync subject type for SvcSvcRule
func (r *SvcSvcRule) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeSvcSvcRules
}

// GetSyncKey returns a unique key for the SvcSvcRule
func (r *SvcSvcRule) GetSyncKey() string {
	if r.Namespace != "" {
		return fmt.Sprintf("svcsvc-rule-%s/%s", r.Namespace, r.Name)
	}
	return fmt.Sprintf("svcsvc-rule-%s", r.Name)
}

// ToSGroupsProto converts domain model to sgroups protobuf
func (r *SvcSvcRule) ToSGroupsProto() (interface{}, error) {
	if r == nil {
		return nil, fmt.Errorf("SvcSvcRule cannot be nil")
	}
	if r.ServiceFromRef.Name == "" || r.ServiceToRef.Name == "" {
		return nil, fmt.Errorf("SvcSvcRule %s/%s missing service references", r.Namespace, r.Name)
	}

	// Build fully-qualified name (namespace/name)
	ruleName := r.Name
	if r.Namespace != "" {
		ruleName = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	}

	// Build service references (namespace/name format)
	svcFromNamespace := r.ServiceFromRef.Namespace
	if svcFromNamespace == "" {
		svcFromNamespace = r.Namespace
	}
	svcFrom := r.ServiceFromRef.Name
	if svcFromNamespace != "" {
		svcFrom = fmt.Sprintf("%s/%s", svcFromNamespace, r.ServiceFromRef.Name)
	}

	svcTo := r.ServiceToRef.Name
	if r.ServiceToRef.Namespace != "" {
		svcTo = fmt.Sprintf("%s/%s", r.ServiceToRef.Namespace, r.ServiceToRef.Name)
	}

	// Convert domain RuleAction to protobuf RuleAction
	var protoAction pb.RuleAction
	switch r.Action {
	case ActionAccept:
		protoAction = pb.RuleAction_ACCEPT
	case ActionDrop:
		protoAction = pb.RuleAction_DROP
	default:
		protoAction = pb.RuleAction_UNDEF
	}

	// Convert priority to RulePriority protobuf
	priority := &pb.RulePriority{
		Value: &pb.RulePriority_Some{Some: r.Priority},
	}

	// Build protobuf message
	return &pb.SvcSvcRule{
		Name:     ruleName,
		SvcFrom:  svcFrom,
		SvcTo:    svcTo,
		Action:   protoAction,
		Priority: priority,
		Logs:     r.Logs,
		Trace:    r.Trace,
	}, nil
}

// XSvcSvcRules - rules where a Service participates (READ-ONLY, populated by DB triggers)
type XSvcSvcRules struct {
	AsServiceFrom []v1beta1.NamespacedObjectReference // Rules where Service is source
	AsServiceTo   []v1beta1.NamespacedObjectReference // Rules where Service is destination
}
