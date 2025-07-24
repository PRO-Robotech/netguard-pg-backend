package models

import (
	"fmt"

	pb "github.com/H-BF/protos/pkg/api/sgroups"
	"github.com/H-BF/protos/pkg/api/common"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkItem represents a network item in an address group
type NetworkItem struct {
	Name       string `json:"name"`
	CIDR       string `json:"cidr"`
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
}

// AddressGroup represents an address group configuration for Netguard
type AddressGroup struct {
	SelfRef
	DefaultAction    RuleAction    `json:"defaultAction"`              // Default action for the address group (ACCEPT/DROP)
	Logs             bool          `json:"logs,omitempty"`             // Whether to enable logs
	Trace            bool          `json:"trace,omitempty"`            // Whether to enable trace
	Networks         []NetworkItem `json:"networks,omitempty"`         // Networks associated with this address group
	AddressGroupName string        `json:"addressGroupName,omitempty"` // Name used in sgroups synchronization
	Meta             Meta
}

// AddressGroupRef represents a reference to an AddressGroup
type AddressGroupRef struct {
	ResourceIdentifier
}

// NewAddressGroupRef creates a new AddressGroupRef
func NewAddressGroupRef(name string, opts ...ResourceIdentifierOption) AddressGroupRef {
	return AddressGroupRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}

// SyncableEntity interface implementation for AddressGroup

// GetSyncSubjectType returns the sync subject type for AddressGroup
func (ag *AddressGroup) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeGroups
}

// GetSyncKey returns a unique key for the AddressGroup
func (ag *AddressGroup) GetSyncKey() string {
	if ag.Namespace != "" {
		return fmt.Sprintf("addressgroup-%s/%s", ag.Namespace, ag.Name)
	}
	return fmt.Sprintf("addressgroup-%s", ag.Name)
}

// ToSGroupsProto converts the AddressGroup to sgroups protobuf format
func (ag *AddressGroup) ToSGroupsProto() (interface{}, error) {
	if ag == nil {
		return nil, fmt.Errorf("AddressGroup cannot be nil")
	}

	// Convert DefaultAction to pb.SecGroup_DefaultAction
	var defaultAction pb.SecGroup_DefaultAction
	switch ag.DefaultAction {
	case ActionDrop:
		defaultAction = pb.SecGroup_DROP
	case ActionAccept:
		defaultAction = pb.SecGroup_ACCEPT
	default:
		// Используем ACCEPT как безопасное значение по умолчанию вместо DEFAULT
		defaultAction = pb.SecGroup_ACCEPT
	}

	// Convert Networks to protobuf format
	var networks []*pb.Network
	for _, network := range ag.Networks {
		networks = append(networks, &pb.Network{
			Name: network.Name,
			Network: &common.Networks_NetIP{
				CIDR: network.CIDR,
			},
		})
	}

	// Convert to real sgroups protobuf format
	protoGroup := &pb.SecGroup{
		Name:          ag.Name,
		Networks:      nil, // Используем пустой список, если нет сетей
		DefaultAction: defaultAction,
		Trace:         ag.Trace,
		Logs:          ag.Logs,
	}

	// Если есть namespace, добавляем его к имени группы
	if ag.Namespace != "" {
		protoGroup.Name = fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)
	}

	return &pb.SyncSecurityGroups{
		Groups: []*pb.SecGroup{protoGroup},
	}, nil
}
