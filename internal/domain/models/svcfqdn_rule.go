package models

import (
	"fmt"

	commonpb "github.com/PRO-Robotech/protos/pkg/api/common"
	sgpb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// FqdnPortSpec represents a pair of source/destination port expressions for SvcFqdnRule
type FqdnPortSpec struct {
	Source      string
	Destination string
}

// SvcFqdnRule - domain model for service-to-FQDN firewall rule
type SvcFqdnRule struct {
	SelfRef
	ServiceFromRef v1beta1.NamespacedObjectReference
	FQDN           string
	Transport      TransportProtocol
	Ports          []FqdnPortSpec
	Logs           bool
	Trace          bool
	Action         RuleAction
	Priority       int32
	Description    string
	Meta           Meta
}

// ServiceFromKey returns "namespace/name" format for map keys and queries
func (r *SvcFqdnRule) ServiceFromKey() string {
	if r.ServiceFromRef.Namespace == "" {
		return r.ServiceFromRef.Name
	}
	return r.ServiceFromRef.Namespace + "/" + r.ServiceFromRef.Name
}

// GetMeta returns K8s metadata
func (r *SvcFqdnRule) GetMeta() Meta {
	return r.Meta
}

// SetMeta sets K8s metadata
func (r *SvcFqdnRule) SetMeta(meta Meta) {
	r.Meta = meta
}

// GetSyncSubjectType returns the sync subject type for SvcFqdnRule
func (r *SvcFqdnRule) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeSvcFqdnRules
}

// GetSyncKey returns a unique key for the SvcFqdnRule
func (r *SvcFqdnRule) GetSyncKey() string {
	if r.Namespace != "" {
		return fmt.Sprintf("svcfqdn-rule-%s/%s", r.Namespace, r.Name)
	}
	return fmt.Sprintf("svcfqdn-rule-%s", r.Name)
}

// ToSGroupsProto converts domain model to sgroups protobuf
func (r *SvcFqdnRule) ToSGroupsProto() (interface{}, error) {
	if r == nil {
		return nil, fmt.Errorf("SvcFqdnRule cannot be nil")
	}
	if r.ServiceFromRef.Name == "" {
		return nil, fmt.Errorf("SvcFqdnRule %s/%s missing serviceFrom reference", r.Namespace, r.Name)
	}
	if r.FQDN == "" {
		return nil, fmt.Errorf("SvcFqdnRule %s/%s missing FQDN", r.Namespace, r.Name)
	}

	ruleName := r.Name
	if r.Namespace != "" {
		ruleName = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	}

	svcFrom := r.ServiceFromRef.Name
	if r.ServiceFromRef.Namespace != "" {
		svcFrom = fmt.Sprintf("%s/%s", r.ServiceFromRef.Namespace, r.ServiceFromRef.Name)
	}

	// Convert transport to protobuf enum
	var transport commonpb.Networks_NetIP_Transport
	switch r.Transport {
	case TCP:
		transport = commonpb.Networks_NetIP_TCP
	case UDP:
		transport = commonpb.Networks_NetIP_UDP
	default:
		transport = commonpb.Networks_NetIP_TCP
	}

	// Convert ports to protobuf AccPorts list
	ports := make([]*sgpb.AccPorts, 0, len(r.Ports))
	for _, p := range r.Ports {
		ports = append(ports, &sgpb.AccPorts{S: p.Source, D: p.Destination})
	}

	// Convert action to protobuf enum
	var protoAction sgpb.RuleAction
	switch r.Action {
	case ActionAccept:
		protoAction = sgpb.RuleAction_ACCEPT
	case ActionDrop:
		protoAction = sgpb.RuleAction_DROP
	default:
		protoAction = sgpb.RuleAction_UNDEF
	}

	protoRule := &sgpb.SvcFqdnRule{
		Name:      ruleName,
		SvcFrom:   svcFrom,
		FQDN:      r.FQDN,
		Transport: transport,
		Ports:     ports,
		Logs:      r.Logs,
		Trace:     r.Trace,
		Action:    protoAction,
		Priority:  &sgpb.RulePriority{Value: &sgpb.RulePriority_Some{Some: r.Priority}},
	}

	return protoRule, nil
}

// XSvcFqdnRules - READ-ONLY metadata for Service resource populated by database triggers
type XSvcFqdnRules struct {
	Rules []ResourceIdentifier
}
