package models

import (
	"fmt"

	commonpb "github.com/PRO-Robotech/protos/pkg/api/common"
	sgpb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// IECidrSvcPortSpec represents access ports (source/destination) for IECidrSvcRule
type IECidrSvcPortSpec struct {
	S string // Source port expression (optional)
	D string // Destination port expression (required)
}

// IECidrSvcRule - domain model for ingress/egress CIDR to service firewall rule
type IECidrSvcRule struct {
	SelfRef
	Transport   TransportProtocol                 // TCP or UDP
	CIDR        string                            // IPv4/IPv6 CIDR notation
	ServiceRef  v1beta1.NamespacedObjectReference // Service reference
	Traffic     Traffic                           // INGRESS or EGRESS
	Ports       []IECidrSvcPortSpec               // Access port set(s)
	Logs        bool
	Trace       bool
	Action      RuleAction // ACCEPT or DROP
	Priority    int32
	Description string
	Comment     string // Optional user comment (Netguard-only, not synced to SGROUPS)
	Meta        Meta
}

// ServiceKey returns "namespace/name" format for map keys and queries
func (r *IECidrSvcRule) ServiceKey() string {
	if r.ServiceRef.Namespace == "" {
		return r.ServiceRef.Name
	}
	return r.ServiceRef.Namespace + "/" + r.ServiceRef.Name
}

// GetMeta returns K8s metadata
func (r *IECidrSvcRule) GetMeta() Meta {
	return r.Meta
}

// SetMeta sets K8s metadata
func (r *IECidrSvcRule) SetMeta(meta Meta) {
	r.Meta = meta
}

// GetSyncSubjectType returns the sync subject type for IECidrSvcRule
func (r *IECidrSvcRule) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIECidrSvcRules
}

// GetSyncKey returns a unique key for the IECidrSvcRule
func (r *IECidrSvcRule) GetSyncKey() string {
	if r.Namespace != "" {
		return fmt.Sprintf("iecidrsvc-rule-%s/%s", r.Namespace, r.Name)
	}
	return fmt.Sprintf("iecidrsvc-rule-%s", r.Name)
}

// ToSGroupsProto converts domain model to sgroups protobuf
func (r *IECidrSvcRule) ToSGroupsProto() (interface{}, error) {
	if r == nil {
		return nil, fmt.Errorf("IECidrSvcRule cannot be nil")
	}
	if r.ServiceRef.Name == "" {
		return nil, fmt.Errorf("IECidrSvcRule %s/%s missing service reference", r.Namespace, r.Name)
	}
	if r.CIDR == "" {
		return nil, fmt.Errorf("IECidrSvcRule %s/%s missing CIDR", r.Namespace, r.Name)
	}

	// Build rule name with namespace prefix
	ruleName := r.Name
	if r.Namespace != "" {
		ruleName = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	}

	// Build service name with namespace
	svc := r.ServiceRef.Name
	if r.ServiceRef.Namespace != "" {
		svc = fmt.Sprintf("%s/%s", r.ServiceRef.Namespace, r.ServiceRef.Name)
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

	// Convert traffic direction to protobuf enum
	var traffic commonpb.Traffic
	switch r.Traffic {
	case INGRESS:
		traffic = commonpb.Traffic_Ingress
	case EGRESS:
		traffic = commonpb.Traffic_Egress
	default:
		traffic = commonpb.Traffic_Undef
	}

	// Convert ports to protobuf AccPorts list
	ports := make([]*sgpb.AccPorts, 0, len(r.Ports))
	for _, p := range r.Ports {
		ports = append(ports, &sgpb.AccPorts{
			S: p.S,
			D: p.D,
		})
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

	protoRule := &sgpb.IECidrSvcRule{
		Name:      ruleName,
		Transport: transport,
		CIDR:      r.CIDR,
		Svc:       svc,
		Traffic:   traffic,
		Ports:     ports,
		Logs:      r.Logs,
		Trace:     r.Trace,
		Action:    protoAction,
		Priority:  &sgpb.RulePriority{Value: &sgpb.RulePriority_Some{Some: r.Priority}},
	}

	return protoRule, nil
}

// XIECidrSvcRules - READ-ONLY metadata for Service resource populated by database triggers
type XIECidrSvcRules struct {
	Rules []ResourceIdentifier
}
