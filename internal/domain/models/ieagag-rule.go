package models

import (
	"fmt"

	"github.com/PRO-Robotech/protos/pkg/api/common"
	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// IEAgAgRule представляет правило между двумя группами адресов
type IEAgAgRule struct {
	SelfRef
	Transport         TransportProtocol
	Traffic           Traffic
	AddressGroupLocal v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind, name and namespace
	AddressGroup      v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind, name and namespace
	Ports             []PortSpec
	Action            RuleAction
	Logs              bool
	Trace             bool // Whether to enable trace
	Priority          int32
	Meta              Meta
}

// AddressGroupLocalKey returns the key for the AddressGroupLocal (namespace/name)
func (r *IEAgAgRule) AddressGroupLocalKey() string {
	if r.AddressGroupLocal.Namespace == "" {
		return r.AddressGroupLocal.Name
	}
	return r.AddressGroupLocal.Namespace + "/" + r.AddressGroupLocal.Name
}

// AddressGroupKey returns the key for the AddressGroup (namespace/name)
func (r *IEAgAgRule) AddressGroupKey() string {
	if r.AddressGroup.Namespace == "" {
		return r.AddressGroup.Name
	}
	return r.AddressGroup.Namespace + "/" + r.AddressGroup.Name
}

// PortSpec определяет спецификацию портов
type PortSpec struct {
	Source      string // Опционально, порт источника
	Destination string // Порт назначения
}

// RuleAction представляет действие правила
type RuleAction string

const (
	ActionAccept RuleAction = "ACCEPT"
	ActionDrop   RuleAction = "DROP"
)

// IEAgAgRuleRef представляет ссылку на IEAgAgRule
type IEAgAgRuleRef struct {
	ResourceIdentifier
}

// NewIEAgAgRuleRef создает новую ссылку на IEAgAgRule
func NewIEAgAgRuleRef(name string, opts ...ResourceIdentifierOption) IEAgAgRuleRef {
	return IEAgAgRuleRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}

// SyncableEntity interface implementation for IEAgAgRule

// GetSyncSubjectType returns the sync subject type for IEAgAgRule
func (r *IEAgAgRule) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIEAgAgRules
}

// GetSyncKey returns a unique key for the IEAgAgRule
func (r *IEAgAgRule) GetSyncKey() string {
	if r.Namespace != "" {
		return fmt.Sprintf("ieagagrule-%s/%s", r.Namespace, r.Name)
	}
	return fmt.Sprintf("ieagagrule-%s", r.Name)
}

// ToSGroupsProto converts the IEAgAgRule to sgroups protobuf format
func (r *IEAgAgRule) ToSGroupsProto() (interface{}, error) {
	if r == nil {
		return nil, fmt.Errorf("IEAgAgRule cannot be nil")
	}

	// Convert Transport
	var transport common.Networks_NetIP_Transport
	switch r.Transport {
	case TCP:
		transport = common.Networks_NetIP_TCP
	case UDP:
		transport = common.Networks_NetIP_UDP
	default:
		transport = common.Networks_NetIP_TCP // default to TCP
	}

	// Convert Traffic
	var traffic common.Traffic
	switch r.Traffic {
	case INGRESS:
		traffic = common.Traffic_Ingress
	case EGRESS:
		traffic = common.Traffic_Egress
	default:
		traffic = common.Traffic_Ingress // default to Ingress
	}

	// Convert Action
	var action pb.RuleAction
	switch r.Action {
	case ActionAccept:
		action = pb.RuleAction_ACCEPT
	case ActionDrop:
		action = pb.RuleAction_DROP
	default:
		action = pb.RuleAction_ACCEPT // default to ACCEPT
	}

	// Convert Ports
	var ports []*pb.AccPorts
	for _, port := range r.Ports {
		if port.Destination != "" {
			ports = append(ports, &pb.AccPorts{
				S: port.Source,      // Source port (can be empty)
				D: port.Destination, // Destination port
			})
		}
	}

	// Build SG and SgLocal with proper namespace handling
	var sg, sgLocal string
	if r.AddressGroup.Namespace != "" {
		sg = fmt.Sprintf("%s/%s", r.AddressGroup.Namespace, r.AddressGroup.Name)
	} else {
		sg = r.AddressGroup.Name
	}

	if r.AddressGroupLocal.Namespace != "" {
		sgLocal = fmt.Sprintf("%s/%s", r.AddressGroupLocal.Namespace, r.AddressGroupLocal.Name)
	} else {
		sgLocal = r.AddressGroupLocal.Name
	}

	// Convert to single sgroups protobuf rule (batch aggregation will be handled by syncer)
	pbRule := &pb.IESgSgRule{
		Transport: transport,
		SG:        sg,      // Remote AddressGroup
		SgLocal:   sgLocal, // Local AddressGroup
		Traffic:   traffic,
		Ports:     ports,
		Logs:      r.Logs,
		Action:    action,
		Trace:     r.Trace,
	}

	// Return single rule element (not wrapped in SyncIESgSgRules)
	// Batch aggregation will be handled by IEAgAgRuleSyncer.SyncBatch()
	return pbRule, nil
}
