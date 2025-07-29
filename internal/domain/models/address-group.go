package models

import (
	"fmt"

	"netguard-pg-backend/internal/sync/types"

	pb "github.com/H-BF/protos/pkg/api/sgroups"
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

	// Convert Networks to network names for SecGroup
	var networkNames []string
	fmt.Printf("🔧 DEBUG: AddressGroup.ToSGroupsProto - Converting %d networks for %s\n", len(ag.Networks), ag.GetSyncKey())
	for i, network := range ag.Networks {
		// Use network.Name as is (already contains namespace)
		fmt.Printf("  🔧 DEBUG: AddressGroup.ToSGroupsProto - network[%d] Name=%s CIDR=%s\n", i, network.Name, network.CIDR)
		networkNames = append(networkNames, network.Name)
	}
	fmt.Printf("🔧 DEBUG: AddressGroup.ToSGroupsProto - Final networkNames: %v\n", networkNames)

	// Convert to real sgroups protobuf format
	protoGroup := &pb.SecGroup{
		Name:          ag.Name,
		Networks:      networkNames, // Используем имена networks
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

// GetID returns the unique identifier for the address group
func (ag *AddressGroup) GetID() string {
	return ag.Key()
}

// GetName returns the name of the address group
func (ag *AddressGroup) GetName() string {
	return ag.Name
}

// GetNamespace returns the namespace of the address group
func (ag *AddressGroup) GetNamespace() string {
	return ag.Namespace
}

// GetMeta returns the metadata of the address group
func (ag *AddressGroup) GetMeta() *Meta {
	return &ag.Meta
}

// Key returns the unique key for the address group (namespace/name)
func (ag *AddressGroup) Key() string {
	if ag.Namespace == "" {
		return ag.Name
	}
	return fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)
}

// GetGeneration returns the generation of the address group
func (ag *AddressGroup) GetGeneration() int64 {
	return ag.Meta.Generation
}

// DeepCopy creates a deep copy of the address group
func (ag *AddressGroup) DeepCopy() Resource {
	if ag == nil {
		return nil
	}

	copy := *ag
	copy.Meta = ag.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices
	if ag.Networks != nil {
		copy.Networks = make([]NetworkItem, len(ag.Networks))
		for i, network := range ag.Networks {
			copy.Networks[i] = network
		}
	}

	return &copy
}
